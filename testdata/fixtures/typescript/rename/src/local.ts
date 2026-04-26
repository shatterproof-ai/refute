export function total(values: number[]): number {
    const totalCount = values.reduce((sum, value) => sum + value, 0);
    return totalCount;
}
