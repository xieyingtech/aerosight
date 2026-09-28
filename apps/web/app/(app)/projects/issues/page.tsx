"use client";
import { IssueList } from "@/components/issue-list";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import type { IssueListItem } from "@/lib/web-api-types";
export default function WorkspacePage() {
  return <StaticAPIPage<IssueListItem[]> endpoint={query => { const id = positiveParam(query); return id ? `/api/projects/${id}/issues` : null; }}>
    {(items, query) => <Page title="案件" description="跟进案件进展、协作处置与关联证据。"><IssueList items={items} projectId={positiveParam(query)!} /></Page>}
  </StaticAPIPage>;
}
