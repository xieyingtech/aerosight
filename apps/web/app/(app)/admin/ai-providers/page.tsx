import { AIProviderForm } from "@/components/ai-provider-form";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { listAIProviders } from "@/lib/ai-providers";

export default async function AdminAIProvidersPage() {
  const providers = await listAIProviders();
  return <Page title="AI Provider" description="平台智能体和 @copilot 使用唯一启用的默认模型配置；API Key 加密保存且不会回显">
    <div className="space-y-6">
      <section className="space-y-3"><h2 className="text-lg font-semibold">新增配置</h2><AIProviderForm /></section>
      <section className="space-y-3"><h2 className="text-lg font-semibold">现有配置</h2>
        {providers.length ? providers.map((provider) => <div className="space-y-3" key={provider.id}>
          <div className="flex flex-wrap items-center gap-2"><span className="font-medium">{provider.name}</span>
            <Badge variant="outline">{provider.providerType}</Badge><Badge variant="outline">{provider.status}</Badge>
            {provider.isDefault ? <Badge>默认</Badge> : null}
          </div><AIProviderForm provider={provider} />
        </div>) : <p className="text-sm text-muted-foreground">尚未配置 AI Provider，智能体功能当前不可用。</p>}
      </section>
    </div>
  </Page>;
}
