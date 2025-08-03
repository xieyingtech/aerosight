export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-echarts",
    "@nuxtjs/leaflet",
    "@vueuse/nuxt",
    "@nuxt/ui",
  ],
  runtimeConfig: {
    public: {
      tiandituApikey: "",
    },
  },
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
});
