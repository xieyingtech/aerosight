// JSON transport serializes database timestamps rather than reviving Date objects.
export type JSONValueOf<T> = T extends Date ? string
  : T extends readonly (infer Item)[] ? JSONValueOf<Item>[]
  : T extends object ? { [Key in keyof T]: JSONValueOf<T[Key]> }
  : T;
