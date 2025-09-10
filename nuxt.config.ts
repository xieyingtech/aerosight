export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-authorization",
    "nuxt-echarts",
    "@vueuse/nuxt",
    "@nuxt/ui",
  ],
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
});
