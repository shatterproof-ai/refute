import { greet } from "./greeter";
import { Person } from "./person";
import type { NamedThing } from "./types";

const message = greet("world");
console.log(message);

const person = new Person("world");
console.log(person.greet());

const thing: NamedThing = { name: "world" };
console.log(thing.name);
