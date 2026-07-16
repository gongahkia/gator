use std::io::{self, Write};

fn main() {
    let mut output = io::stdout().lock();
    writeln!(output, "gator-index 0.1.0").expect("write indexer version");
}

#[cfg(test)]
mod tests {
    #[test]
    fn version_is_present() {
        assert!(!env!("CARGO_PKG_VERSION").is_empty());
    }
}
