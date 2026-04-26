export class Person {
    constructor(public readonly name: string) {}

    greet(): string {
        return `Hi, ${this.name}!`;
    }
}
