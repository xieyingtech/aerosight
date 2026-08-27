#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

import { validateDeviceNetworkProfile } from "../../apps/web/lib/device-network-profile.ts";

const files = process.argv.slice(2);
if (!files.length) {
  files.push("infra/demo/profiles/lan.example.json", "infra/demo/profiles/public.example.json");
}

const documentationResolver = async (hostname) => {
  if (hostname.endsWith(".example.test")) return [{ address: "8.8.8.8", family: 4 }];
  throw new Error("example validation only resolves .example.test placeholders");
};

let failed = false;
for (const file of files) {
  const absolutePath = resolve(file);
  const profile = JSON.parse(await readFile(absolutePath, "utf8"));
  const result = await validateDeviceNetworkProfile(profile, { resolver: documentationResolver });
  if (result.valid) {
    process.stdout.write(`valid network profile: ${file} (${profile.mode})\n`);
    continue;
  }
  failed = true;
  process.stderr.write(`invalid network profile: ${file}\n`);
  for (const issue of result.issues) process.stderr.write(`  ${issue.field}: ${issue.code}\n`);
}

if (failed) process.exitCode = 1;
