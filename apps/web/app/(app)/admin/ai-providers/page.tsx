"use client";

import { AIProviderForm } from "@/components/ai-provider-form";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import type { AIProviderView } from "@/lib/ai-providers";

export default function AdminAIProvidersPage() {
  const state = useAPI<AIProviderView[]>("/api/admin/ai-providers");
  return <APIStateView state={state}>{(providers) => <Page title="AI Provider" description="平台智能体和 @copilot 使用唯一启用的默认模型配置；API Key 加密保存且不会回显">
    <div className="space-y-6">
      <section className="space-y-3"><h2 className="text-lg font-semibold">新增配置</h2><AIProviderForm onChanged={state.reload} /></section>
      <section className="space-y-3"><h2 className="text-lg font-semibold">现有配置</h2>
        {providers.length ? providers.map((provider) => <div className="space-y-3" key={provider.id}>
          <div className="flex flex-wrap items-center gap-2"><span className="font-medium">{provider.name}</span>
            <Badge variant="outline">{provider.providerType}</Badge><Badge variant="outline">{provider.status}</Badge>
            {provider.isDefault ? <Badge>默认</Badge> : null}
          </div><AIProviderForm provider={provider} onChanged={state.reload} />
        </div>) : <p className="text-sm text-muted-foreground">尚未配置 AI Provider，智能体功能当前不可用。</p>}
      </section>
    </div>
  </Page>}</APIStateView>;
}

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
