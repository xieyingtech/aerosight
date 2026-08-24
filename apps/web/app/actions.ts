"use server";

import { AuthError } from "next-auth";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { z } from "zod";
import { signIn, signOut } from "@/auth";
import { createProject, createTeam } from "@/lib/data";

export type FormState = { error?: string };

export async function loginAction(_state: FormState, formData: FormData): Promise<FormState> {
  try {
    await signIn("credentials", {
      username: formData.get("username"),
      password: formData.get("password"),
      redirectTo: "/projects"
    });
    return {};
  } catch (error) {
    if (error instanceof AuthError) return { error: "邮箱、手机号或密码错误" };
    throw error;
  }
}

export async function logoutAction() {
  await signOut({ redirectTo: "/login" });
}

const teamSchema = z.object({ name: z.string().trim().min(1, "请输入团队名称").max(100) });

export async function createTeamAction(_state: FormState, formData: FormData): Promise<FormState> {
  const parsed = teamSchema.safeParse({ name: formData.get("name") });
  if (!parsed.success) return { error: parsed.error.issues[0]?.message };
  await createTeam(parsed.data.name);
  revalidatePath("/teams");
  redirect("/teams");
}

const projectSchema = z.object({
  teamId: z.coerce.number().int().positive("请选择团队"),
  name: z.string().trim().min(1, "请输入项目名称").max(100)
});

export async function createProjectAction(_state: FormState, formData: FormData): Promise<FormState> {
  const parsed = projectSchema.safeParse({ teamId: formData.get("teamId"), name: formData.get("name") });
  if (!parsed.success) return { error: parsed.error.issues[0]?.message };
  const project = await createProject(parsed.data.teamId, parsed.data.name);
  revalidatePath("/projects");
  redirect(`/projects/${project.id}`);
}
