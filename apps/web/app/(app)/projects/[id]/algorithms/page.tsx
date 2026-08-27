import { AlgorithmProviderForm } from "@/components/algorithm-provider-form";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { listAlgorithmProviders } from "@/lib/algorithm-providers";

export default async function AlgorithmsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const providers = await listAlgorithmProviders(projectId);
  return <Page title="算法服务" description="仅保存秘密引用，连接凭据不会回显"><div className="space-y-4">
    <AlgorithmProviderForm projectId={projectId} />
    <div className="grid gap-3 md:grid-cols-2">{providers.length ? providers.map((provider) => <Card key={String(provider.id)}><CardHeader><CardTitle>{provider.name}</CardTitle></CardHeader><CardContent className="space-y-2 text-sm"><div className="flex gap-2"><Badge variant="outline">{provider.providerType}</Badge><Badge variant="outline">{provider.status}</Badge></div><p className="truncate text-muted-foreground">{provider.baseUrl}</p><p>{provider.secretConfigured ? "秘密引用已配置" : "未配置认证秘密"}</p></CardContent></Card>) : <p className="text-sm text-muted-foreground">尚未配置算法服务</p>}</div>
  </div></Page>;
}
