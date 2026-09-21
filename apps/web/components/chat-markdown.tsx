import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

const components: Components = {
  a: ({ href, children }) => href
    ? <a href={href} target={/^(https?:)?\/\//i.test(href) ? "_blank" : undefined} rel="noopener noreferrer" className="text-primary underline underline-offset-4">{children}</a>
    : <span>{children}</span>,
  table: ({ children }) => <div className="my-4 max-w-full overflow-x-auto" tabIndex={0} role="region" aria-label="回复表格"><table>{children}</table></div>,
  pre: ({ children }) => <pre tabIndex={0}>{children}</pre>,
};

export function ChatMarkdown({ content }: { content: string }) {
  return <div className="chat-markdown min-w-0 text-sm leading-7">
    <Markdown remarkPlugins={[remarkGfm]} components={components}>{content}</Markdown>
  </div>;
}
