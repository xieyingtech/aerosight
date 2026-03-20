export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
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
  hub: {
    db: "postgresql",
  },
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
});
