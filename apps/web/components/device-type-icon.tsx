"use client";

import { CpuIcon, type LucideProps } from "lucide-react";
import { DynamicIcon, iconNames, type IconName } from "lucide-react/dynamic";

const supportedIcons = new Set<string>(iconNames);

// DeviceType owns the icon name; missing or unsupported definitions use one neutral icon.
export function DeviceTypeIcon({ name, ...props }: Omit<LucideProps, "name"> & { name?: string | null }) {
  if (!name || !supportedIcons.has(name)) return <CpuIcon aria-hidden="true" {...props} />;
  return <DynamicIcon name={name as IconName} aria-hidden="true" {...props} />;
}
