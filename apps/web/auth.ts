import NextAuth from "next-auth";
import Credentials from "next-auth/providers/credentials";
import { compare } from "bcryptjs";
import { z } from "zod";
import { query } from "@/lib/db";

const credentialsSchema = z.object({
  username: z.string().trim().min(1),
  password: z.string().min(1)
});

export const { handlers, auth, signIn, signOut } = NextAuth({
  secret: process.env.AUTH_SECRET,
  trustHost: true,
  pages: { signIn: "/login" },
  session: { strategy: "jwt" },
  providers: [
    Credentials({
      credentials: {
        username: { label: "邮箱或手机号", type: "text" },
        password: { label: "密码", type: "password" }
      },
      async authorize(rawCredentials) {
        const parsed = credentialsSchema.safeParse(rawCredentials);
        if (!parsed.success) return null;

        const { username, password } = parsed.data;
        const result = await query<{
          id: number;
          name: string;
          email: string | null;
          password: string | null;
          role: "user" | "admin";
        }>(
          `select id, name, email, password, role
           from users
           where lower(email) = lower($1) or phone = $1
           limit 1`,
          [username]
        );
        const user = result.rows[0];
        if (!user?.password || !(await compare(password, user.password))) return null;

        return {
          id: String(user.id),
          name: user.name,
          email: user.email,
          role: user.role
        };
      }
    })
  ],
  callbacks: {
    jwt({ token, user }) {
      if (user) {
        token.userId = Number(user.id);
        token.role = user.role;
      }
      return token;
    },
    session({ session, token }) {
      session.user.userId = Number(token.userId);
      session.user.role = token.role;
      return session;
    }
  }
});
