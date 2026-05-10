pub mod util;

use std::fmt;

pub fn format_greeting(name: &str) -> String {
    let prefix = "Hello";
    format!("{}, {}!", prefix, name)
}

pub fn compute(x: i32) -> i32 {
    (x * 2) + (x * 2)
}

pub struct Greeter {
    pub name: String,
}

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
