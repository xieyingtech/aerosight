export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-echarts",
    "@unocss/nuxt",
    "@element-plus/nuxt",
    "@nuxtjs/leaflet",
  ],
  app: {
    head: {
      title: "协盈穹目",
    },
  },
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
  runtimeConfig: {
    public: {
      tiandituApikey: "",
    },
  },
});
