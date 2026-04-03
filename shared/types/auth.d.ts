import { users } from "@nuxthub/db/schema";

declare module "#auth-utils" {
  interface User {
    id: number;
    name: string;
    role: (typeof users.role.enumValues)[number];
  }
}

export {};
