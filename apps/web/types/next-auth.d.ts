import "next-auth";
import "next-auth/jwt";

declare module "next-auth" {
  interface User {
    role: "user" | "admin";
  }

  interface Session {
    user: {
      userId: number;
      name: string;
      email?: string | null;
      role: "user" | "admin";
    };
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    userId: number;
    role: "user" | "admin";
  }
}
