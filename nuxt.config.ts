export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "@nuxt/ui",
    "@nuxtjs/supabase",
    "@vueuse/nuxt",
    "nuxt-echarts",
    "nuxt-maplibre",
  ],
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
});