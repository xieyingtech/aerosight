declare module "#auth-utils" {
  interface User {
    id: number;
    username: string;
    admin: boolean;
  }
}

export {};
