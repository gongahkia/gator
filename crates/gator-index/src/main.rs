use serde::Deserialize;
use serde_json::{json, Value};
use std::io::{self, BufRead, Write};

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

fn handle(request: Request) -> Value {
    if request.version != PROTOCOL_VERSION {
        return error(Some(&request.id), "unsupported protocol version");
    }
    let result = match request.method.as_str() {
        "health" => Ok(json!({"status": "healthy", "protocol_version": PROTOCOL_VERSION})),
        "index" => Ok(json!({"accepted": true, "request": request.params})),
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
}
