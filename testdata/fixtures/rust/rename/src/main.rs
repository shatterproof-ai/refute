fn main() {
    let msg = greet::format_greeting("world");
    println!("{}", msg);
    let _g = greet::Greeter { name: "world".to_string() };
}
