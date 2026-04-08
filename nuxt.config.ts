export default defineNuxtConfig({
  compatibilityDate: "2025-07-15",
  modules: [
    "@nuxt/ui",
    "@vueuse/nuxt",
    "nuxt-echarts",
    "nuxt-maplibre",
    "@nuxthub/core",
    "nuxt-auth-utils",
    "@nuxtjs/i18n",
  ],
  ui: {
    fonts: false,
  },
  i18n: {
    defaultLocale: "zh_cn",
    locales: [{ code: "zh_cn", file: "zh_cn.json" }],
  },
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
  hub: {
    db: "postgresql",
  },
  routeRules: {
    "/projects": { appLayout: "default" },
    "/projects/new": { appLayout: "default" },
    "/projects/**": { appLayout: "projects" },
    "/admin/**": { appLayout: "admin" },
  },
});
