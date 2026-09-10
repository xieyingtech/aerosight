import { createHash } from 'node:crypto';

export function pageScriptHashes(html) {
  // Next export emits complete HTML. Match quoted attributes as units so a
  // greater-than character inside an attribute cannot truncate the start tag.
  const scripts = /<script\b(?:[^"'<>]|"[^"]*"|'[^']*')*>([\s\S]*?)<\/script\s*>/gi;
  return [...new Set([...html.matchAll(scripts)].map(match => match[1])
    .filter(body => body.length > 0)
    .map(body => `sha256-${createHash('sha256').update(body.replaceAll('\r\n', '\n').replaceAll('\r', '\n')).digest('base64')}`))].sort();
}

export function pageCSPEntry(bytes) {
  return { digest: createHash('sha256').update(bytes).digest('hex'), hashes: pageScriptHashes(bytes.toString('utf8')) };
}
