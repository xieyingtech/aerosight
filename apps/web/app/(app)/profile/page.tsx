"use client";

import { Page } from "@/components/page";
import { NameSettingsForm } from "@/components/name-settings-form";
import { PasswordSettingsForm } from "@/components/password-settings-form";
import { useSessionUser } from "@/components/session-provider";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function ProfilePage() {
  const user = useSessionUser();
  if (!user) return null;
  return <Page description="管理个人资料和账号安全" title="个人中心">
    <div className="w-full max-w-3xl space-y-5">
      <Card><CardHeader><CardTitle>个人资料</CardTitle></CardHeader><CardContent>
        <dl className="divide-y">
          <div className="flex flex-wrap items-center justify-between gap-4 pb-5"><div className="min-w-0"><dt className="text-sm text-muted-foreground">显示名称</dt><dd className="mt-1 break-words font-medium">{user.name}</dd></div><NameSettingsForm name={user.name} /></div>
          <div className="py-5"><dt className="text-sm text-muted-foreground">登录邮箱</dt><dd className="mt-1 break-words">{user.email ?? "未设置邮箱"}</dd></div>
          {user.phone && <div className="pt-5"><dt className="text-sm text-muted-foreground">手机号</dt><dd className="mt-1">{user.phone}</dd></div>}
        </dl>
      </CardContent></Card>
      <Card><CardHeader><CardTitle>账号安全</CardTitle></CardHeader><CardContent><div className="flex flex-wrap items-center justify-between gap-4"><div><p className="font-medium">登录密码</p><p className="mt-1 text-sm text-muted-foreground">使用原密码验证后更换密码。</p></div><PasswordSettingsForm /></div></CardContent></Card>
    </div>
  </Page>;
}
