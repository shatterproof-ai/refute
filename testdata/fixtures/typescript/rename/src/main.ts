import { greet } from "./greeter";
import { Person } from "./person";

const message = greet("world");
console.log(message);
const p = new Person("world");
console.log(p.greet());
