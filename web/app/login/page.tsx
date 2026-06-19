import { LoginForm } from "./login-form";

export default function LoginPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 px-4">
      <section className="w-full max-w-sm rounded-md border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-xl font-semibold">登录到 AeroSight</h1>
        <p className="mt-1 text-sm text-slate-500">使用邮箱或手机号登录</p>
        <LoginForm />
      </section>
    </main>
  );
}
