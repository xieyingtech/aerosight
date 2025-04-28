export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-echarts",
    "@element-plus/nuxt",
    "@nuxtjs/leaflet",
    "@unocss/nuxt",
    "@vueuse/nuxt",
  ],
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
  runtimeConfig: {
    public: {
      tiandituApikey: "",
    },
  },
  ssr: false,
});
