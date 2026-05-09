pub fn format_greeting(name: &str) -> String {
    format!("Hello, {}!", name)
}

pub struct Greeter {
    pub name: String,
}

use std::fmt;

impl fmt::Display for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Display: {}", self.name)
    }
}

impl fmt::Debug for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Debug: {}", self.name)
    }
}
