export type InputSelectOption = {
  value: string;
  label: string;
  description?: string;
  keywords?: string[];
};

export function filterInputSelectOptions(options: InputSelectOption[], query: string) {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) return options;
  return options.filter((option) => [option.label, option.description, ...(option.keywords ?? [])]
    .some((value) => value?.toLocaleLowerCase().includes(needle)));
}
