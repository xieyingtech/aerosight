import { LoginForm } from "./login-form";
import { auth } from "@/auth";
import { redirect } from "next/navigation";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default async function LoginPage() {
  if (await auth()) redirect("/projects");
  return (
    <main className="flex min-h-screen items-center justify-center bg-muted px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>登录到 AeroSight</CardTitle>
          <CardDescription>空地一体化智能感知平台</CardDescription>
        </CardHeader>
        <CardContent><LoginForm /></CardContent>
      </Card>
    </main>
  );
}
