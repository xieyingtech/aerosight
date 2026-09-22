"use client";

import { CheckIcon, ChevronsUpDownIcon, SearchIcon } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";

import { filterInputSelectOptions, type InputSelectOption } from "@/lib/input-select-core";
import { cn } from "@/lib/utils";

export function InputSelect({ value, options, onValueChange, placeholder = "搜索并选择", allowCustom = false, ariaLabel, disabled = false, emptyMessage = "没有匹配的设备" }: {
  value: string | null;
  options: InputSelectOption[];
  onValueChange: (value: string) => void;
  placeholder?: string;
  allowCustom?: boolean;
  ariaLabel?: string;
  disabled?: boolean;
  emptyMessage?: string;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const listId = useId();
  const selected = options.find((option) => option.value === value) ?? null;
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState(selected?.label ?? (allowCustom ? value ?? "" : ""));
  const [activeIndex, setActiveIndex] = useState(0);
  const filtered = useMemo(() => {
    const matches = filterInputSelectOptions(options, open ? query : "");
    const custom = query.trim();
    return allowCustom && custom && !options.some((option) => option.value === custom)
      ? [...matches, { value: custom, label: custom, description: "添加自定义模型" }]
      : matches;
  }, [open, options, query, allowCustom]);

  useEffect(() => { if (!open) setQuery(selected?.label ?? (allowCustom ? value ?? "" : "")); }, [open, selected?.label, allowCustom, value]);
  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);

  useEffect(() => {
    if (open) document.getElementById(`${listId}-${activeIndex}`)?.scrollIntoView({ block: "nearest" });
  }, [open, listId, activeIndex]);

  const choose = (option: InputSelectOption) => {
    onValueChange(option.value);
    setQuery(option.label);
    setOpen(false);
  };

  return <div className="relative" ref={rootRef}>
    <SearchIcon className="pointer-events-none absolute left-3 top-2.5 z-10 size-4 text-muted-foreground" />
    <input
      aria-label={ariaLabel}
      disabled={disabled}
      aria-activedescendant={open && filtered[activeIndex] ? `${listId}-${activeIndex}` : undefined}
      aria-autocomplete="list"
      aria-controls={listId}
      aria-expanded={open}
      className="flex h-9 w-full rounded-lg border border-input bg-transparent py-1 pl-9 pr-9 text-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
      onChange={(event) => { setQuery(event.target.value); setActiveIndex(0); setOpen(true); }}
      onFocus={(event) => { setOpen(true); event.currentTarget.select(); }}
      onKeyDown={(event) => {
        if (event.key === "ArrowDown") { event.preventDefault(); setOpen(true); setActiveIndex((current) => open ? Math.max(0, Math.min(filtered.length - 1, current + 1)) : 0); }
        if (event.key === "ArrowUp") { event.preventDefault(); setActiveIndex((current) => Math.max(0, current - 1)); }
        if (event.key === "Enter") { event.preventDefault(); if (open && filtered[activeIndex]) choose(filtered[activeIndex]); }
        if (event.key === "Escape" && open) { event.preventDefault(); event.stopPropagation(); setOpen(false); setQuery(selected?.label ?? (allowCustom ? value ?? "" : "")); }
      }}
      placeholder={placeholder}
      role="combobox"
      value={query}
    />
    <ChevronsUpDownIcon className="pointer-events-none absolute right-3 top-2.5 size-4 text-muted-foreground" />
    {open && <div className="absolute z-50 mt-1 max-h-72 w-full overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-md" id={listId} role="listbox">
      {filtered.map((option, index) => <button
        id={`${listId}-${index}`}
        aria-selected={index === activeIndex}
        className={cn("flex w-full items-center gap-2 rounded-md px-2 py-2 text-left", index === activeIndex && "bg-accent")}
        key={option.value}
        onMouseDown={(event) => event.preventDefault()}
        onMouseEnter={() => setActiveIndex(index)}
        onClick={() => choose(option)}
        role="option"
        type="button"
      >
        <CheckIcon className={cn("size-4 shrink-0", option.value === value ? "opacity-100" : "opacity-0")} />
        <span className="min-w-0"><span className="block truncate text-sm font-medium">{option.label}</span>{option.description && <span className="block truncate text-xs text-muted-foreground">{option.description}</span>}</span>
      </button>)}
      {!filtered.length && <p className="px-3 py-6 text-center text-sm text-muted-foreground">{emptyMessage}</p>}
    </div>}
  </div>;
}
