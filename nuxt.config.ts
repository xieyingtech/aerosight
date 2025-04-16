export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-echarts",
    "@unocss/nuxt",
    "@element-plus/nuxt",
    "@nuxtjs/leaflet",
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
