use serde::Deserialize;
use serde_json::{json, Value};
use std::collections::HashSet;
use std::io::{self, BufRead, Write};
use std::path::Path;
use std::process::{Command, Stdio};

const PROTOCOL_VERSION: u32 = 1;

#[derive(Deserialize)]
struct Request {
    version: u32,
    id: String,
    method: String,
    #[serde(default)]
    params: Value,
}

fn error(id: Option<&str>, message: &str) -> Value {
    json!({"version": PROTOCOL_VERSION, "id": id, "ok": false, "error": message})
}

fn approved(params: &Value, name: &str) -> Result<HashSet<String>, &'static str> {
    params
        .get(name)
        .and_then(Value::as_array)
        .ok_or("index requires explicit approved file lists")?
        .iter()
        .map(|value| {
            value
                .as_str()
                .filter(|path| !path.is_empty())
                .map(str::to_owned)
                .ok_or("approved file paths must be non-empty strings")
        })
        .collect()
}

fn git_files(root: &Path, args: &[&str]) -> Result<Vec<String>, &'static str> {
    let output = Command::new("git")
        .args(args)
        .current_dir(root)
        .output()
        .map_err(|_| "Git is unavailable")?;
    if !output.status.success() {
        return Err("Git file scan failed");
    }
    String::from_utf8(output.stdout)
        .map_err(|_| "Git file scan returned invalid UTF-8")?
        .split('\0')
        .filter(|path| !path.is_empty())
        .map(str::to_owned)
        .collect::<Vec<_>>()
        .into_iter()
        .map(|path| {
            if Path::new(&path).is_relative() && !path.split('/').any(|part| part == "..") {
                Ok(path)
            } else {
                Err("Git returned unsafe path")
            }
        })
        .collect()
}

fn index(params: &Value) -> Result<Value, &'static str> {
    let root = params
        .get("root")
        .and_then(Value::as_str)
        .filter(|root| !root.is_empty())
        .ok_or("index requires params.root")?;
    let root = Path::new(root)
        .canonicalize()
        .map_err(|_| "index root is unavailable")?;
    if !root.is_dir() {
        return Err("index root is unavailable");
    }
    let tracked = approved(params, "approved_tracked")?;
    let untracked = approved(params, "approved_untracked")?;
    let mut files: Vec<String> = git_files(&root, &["ls-files", "-z"])?
        .into_iter()
        .filter(|path| tracked.contains(path))
        .collect();
    files.extend(
        git_files(&root, &["ls-files", "-o", "--exclude-standard", "-z"])?
            .into_iter()
            .filter(|path| untracked.contains(path)),
    );
    files.sort();
    files.dedup();
    Ok(json!({"files": files}))
}

fn chunks(params: &Value) -> Result<Value, &'static str> {
    let text = params
        .get("text")
        .and_then(Value::as_str)
        .ok_or("chunk requires params.text")?;
    let language = params
        .get("language")
        .and_then(Value::as_str)
        .unwrap_or("text");
    let lines: Vec<&str> = text.lines().collect();
    let spans = params
        .get("tree_sitter_spans")
        .and_then(Value::as_array)
        .filter(|spans| !spans.is_empty());
    let chunks: Vec<Value> = if let Some(spans) = spans {
        spans.iter().map(|span| {
            let start = span.get("start_line").and_then(Value::as_u64).ok_or("Tree-sitter span requires start_line")? as usize;
            let end = span.get("end_line").and_then(Value::as_u64).ok_or("Tree-sitter span requires end_line")? as usize;
            if start == 0 || end < start || end > lines.len() { return Err("Tree-sitter span is outside document"); }
            Ok(json!({"id": format!("{}:{}:{}", language, start, end), "source": "tree-sitter", "start_line": start, "end_line": end, "text": lines[start - 1..end].join("\n")}))
        }).collect::<Result<_, _>>()?
    } else {
        lines.chunks(32).enumerate().map(|(index, chunk)| {
            let start = index * 32 + 1; let end = start + chunk.len() - 1;
            json!({"id": format!("{}:{}:{}", language, start, end), "source": "text", "start_line": start, "end_line": end, "text": chunk.join("\n")})
        }).collect()
    };
    Ok(json!({"chunks": chunks}))
}

fn embed(params: &Value) -> Result<Value, &'static str> {
    let input = params
        .get("input")
        .and_then(Value::as_str)
        .ok_or("embed requires params.input")?;
    let command = params
        .get("command")
        .and_then(Value::as_array)
        .ok_or("embed requires a local command array")?;
    let argv: Vec<&str> = command
        .iter()
        .map(|part| {
            part.as_str()
                .filter(|part| !part.is_empty())
                .ok_or("embedding command arguments must be strings")
        })
        .collect::<Result<_, _>>()?;
    let executable = Path::new(*argv.first().ok_or("embed requires a local command array")?);
    if !executable.is_absolute() || !executable.is_file() {
        return Err("embedding executable must be an existing absolute local file");
    }
    let mut child = Command::new(executable)
        .args(&argv[1..])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .spawn()
        .map_err(|_| "local embedding provider failed to start")?;
    child
        .stdin
        .as_mut()
        .ok_or("local embedding provider has no stdin")?
        .write_all(input.as_bytes())
        .map_err(|_| "local embedding provider input failed")?;
    let output = child
        .wait_with_output()
        .map_err(|_| "local embedding provider failed")?;
    if !output.status.success() {
        return Err("local embedding provider failed");
    }
    let vector: Vec<f64> = serde_json::from_slice(&output.stdout)
        .map_err(|_| "local embedding provider returned invalid JSON")?;
    if vector.is_empty() || vector.iter().any(|value| !value.is_finite()) {
        return Err("local embedding provider returned invalid vector");
    }
    Ok(json!({"provider":"local-command","vector":vector}))
}

fn handle(request: Request) -> Value {
    if request.version != PROTOCOL_VERSION {
        return error(Some(&request.id), "unsupported protocol version");
    }
    let result = match request.method.as_str() {
        "health" => Ok(json!({"status": "healthy", "protocol_version": PROTOCOL_VERSION})),
        "index" => index(&request.params),
        "chunk" => chunks(&request.params),
        "embed" => embed(&request.params),
        "cancel" => request
            .params
            .get("request_id")
            .and_then(Value::as_str)
            .filter(|id| !id.is_empty())
            .map(|id| json!({"cancelled": id}))
            .ok_or("cancel requires params.request_id"),
        _ => Err("unknown method"),
    };
    match result {
        Ok(result) => {
            json!({"version": PROTOCOL_VERSION, "id": request.id, "ok": true, "result": result})
        }
        Err(message) => error(Some(&request.id), message),
    }
}

fn main() {
    let stdin = io::stdin();
    let mut output = io::stdout().lock();
    for line in stdin.lock().lines() {
        let response = match line {
            Ok(line) if !line.trim().is_empty() => match serde_json::from_str::<Request>(&line) {
                Ok(request) => handle(request),
                Err(_) => error(None, "invalid request"),
            },
            Ok(_) => continue,
            Err(_) => error(None, "input read failed"),
        };
        writeln!(output, "{}", response).expect("write protocol response");
        output.flush().expect("flush protocol response");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn health_is_versioned() {
        let response = handle(Request {
            version: 1,
            id: "one".into(),
            method: "health".into(),
            params: Value::Null,
        });
        assert_eq!(response["ok"], true);
        assert_eq!(response["result"]["protocol_version"], 1);
    }

    #[test]
    fn cancellation_requires_identity() {
        let response = handle(Request {
            version: 1,
            id: "one".into(),
            method: "cancel".into(),
            params: json!({}),
        });
        assert_eq!(response["ok"], false);
    }

    #[test]
    fn approved_lists_require_strings() {
        assert!(approved(&json!({"approved_tracked": ["a"]}), "approved_tracked").is_ok());
        assert!(approved(&json!({"approved_tracked": [1]}), "approved_tracked").is_err());
    }

    #[test]
    fn index_filters_git_files_by_policy() {
        let root = std::env::temp_dir().join(format!("gator-index-{}", std::process::id()));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).unwrap();
        assert!(Command::new("git")
            .args(["init", "-q"])
            .current_dir(&root)
            .status()
            .unwrap()
            .success());
        fs::write(root.join("tracked.txt"), "tracked").unwrap();
        fs::write(root.join("selected.txt"), "selected").unwrap();
        fs::write(root.join("ignored.txt"), "ignored").unwrap();
        fs::write(root.join(".gitignore"), "ignored.txt\n").unwrap();
        assert!(Command::new("git")
            .args(["add", "tracked.txt"])
            .current_dir(&root)
            .status()
            .unwrap()
            .success());
        let result = index(&json!({"root": root, "approved_tracked": ["tracked.txt"], "approved_untracked": ["selected.txt", "ignored.txt"]})).unwrap();
        assert_eq!(result["files"], json!(["selected.txt", "tracked.txt"]));
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn chunks_prefer_tree_sitter_spans() {
        let parsed = chunks(&json!({"language":"lua","text":"one\ntwo\nthree","tree_sitter_spans":[{"start_line":2,"end_line":3}]})).unwrap();
        assert_eq!(parsed["chunks"][0]["source"], "tree-sitter");
        let fallback = chunks(&json!({"text":"one\ntwo"})).unwrap();
        assert_eq!(fallback["chunks"][0]["source"], "text");
    }

    #[test]
    fn embedding_requires_absolute_executable() {
        assert!(embed(&json!({"input":"text","command":["provider"]})).is_err());
    }
}
