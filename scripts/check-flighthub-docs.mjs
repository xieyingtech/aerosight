import { readFile, stat } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const contractDir = resolve(root, "contracts/dji-flighthub/v2");
const documents = [
  resolve(contractDir, "README.md"),
  resolve(contractDir, "CAPABILITY-COVERAGE.md"),
  resolve(root, "docs/operations/dji-flighthub-runbook.md"),
  resolve(root, "docs/operations/dji-flighthub-field-acceptance.md")
];
const requiredColumns = [
  "id", "method", "path", "status", "title", "domain", "scope", "risk",
  "pagination", "deployment", "verification"
];
const expectedMethods = new Map([
  ["GET", 59],
  ["POST", 19],
  ["PUT", 6],
  ["DELETE", 5]
]);

function fail(message) {
  throw new Error(`FlightHub docs check failed: ${message}`);
}

function parseTSV(source) {
  const lines = source.trim().split(/\r?\n/);
  const headers = lines.shift()?.split("\t") ?? [];
  for (const column of requiredColumns) {
    if (!headers.includes(column)) fail(`manifest is missing column ${column}`);
  }
  return lines.map((line, lineIndex) => {
    const values = line.split("\t");
    if (values.length !== headers.length) fail(`manifest line ${lineIndex + 2} has ${values.length} fields, expected ${headers.length}`);
    return Object.fromEntries(headers.map((header, index) => [header, values[index]]));
  });
}

async function checkRelativeLinks(file, source) {
  const links = [...source.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)].map((match) => match[1].trim());
  for (const link of links) {
    if (/^(?:[a-z][a-z0-9+.-]*:|#)/i.test(link)) continue;
    const target = link.replace(/^<|>$/g, "").split("#", 1)[0];
    if (!target) continue;
    try {
      await stat(resolve(dirname(file), decodeURIComponent(target)));
    } catch {
      fail(`${file.slice(root.length + 1)} contains missing relative link ${link}`);
    }
  }
}

const manifestSource = await readFile(resolve(contractDir, "endpoints.tsv"), "utf8");
const endpoints = parseTSV(manifestSource);
if (endpoints.length !== 89) fail(`manifest contains ${endpoints.length} endpoints, expected 89`);

const methodCounts = new Map();
const domainCounts = new Map();
const domains = new Set();
const ids = new Set();
const routes = new Set();
for (const [index, endpoint] of endpoints.entries()) {
  for (const column of requiredColumns) {
    if (!endpoint[column]?.trim()) fail(`manifest line ${index + 2} has empty ${column}`);
  }
  if (endpoint.status !== "released") fail(`endpoint ${endpoint.id} is not released`);
  if (!expectedMethods.has(endpoint.method)) fail(`endpoint ${endpoint.id} has unsupported method ${endpoint.method}`);
  if (!endpoint.path.startsWith("/openapi/v2.0/")) fail(`endpoint ${endpoint.id} has unexpected path ${endpoint.path}`);
  if (ids.has(endpoint.id)) fail(`duplicate endpoint id ${endpoint.id}`);
  const route = `${endpoint.method} ${endpoint.path}`;
  if (routes.has(route)) fail(`duplicate endpoint route ${route}`);
  ids.add(endpoint.id);
  routes.add(route);
  domains.add(endpoint.domain);
  domainCounts.set(endpoint.domain, (domainCounts.get(endpoint.domain) ?? 0) + 1);
  methodCounts.set(endpoint.method, (methodCounts.get(endpoint.method) ?? 0) + 1);
}

for (const [method, expected] of expectedMethods) {
  const actual = methodCounts.get(method) ?? 0;
  if (actual !== expected) fail(`${method} count is ${actual}, expected ${expected}`);
}

const sources = new Map();
for (const document of documents) {
  const source = await readFile(document, "utf8");
  sources.set(document, source);
  await checkRelativeLinks(document, source);
}

const coverage = sources.get(resolve(contractDir, "CAPABILITY-COVERAGE.md"));
for (const domain of domains) {
  const marker = `<!-- domain:${domain} endpoints:${domainCounts.get(domain)} -->`;
  if (!coverage.includes(marker)) fail(`coverage table has no exact-count row for manifest domain ${domain}`);
}
const coverageMarkers = [...coverage.matchAll(/<!-- domain:([a-z-]+) endpoints:(\d+) -->/g)].map((match) => match[1]);
if (new Set(coverageMarkers).size !== coverageMarkers.length) fail("coverage table contains duplicate domain markers");
for (const domain of coverageMarkers) {
  if (!domains.has(domain)) fail(`coverage table contains unknown domain ${domain}`);
}

console.info(`FlightHub docs check passed: ${endpoints.length} released endpoints, ${domains.size} domains, 59 GET / 19 POST / 6 PUT / 5 DELETE, no orphaned domains or broken relative links.`);
