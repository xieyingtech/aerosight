export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: ["@nuxt/ui", "@nuxtjs/supabase", "@vueuse/nuxt", "nuxt-echarts"],
  echarts: {
    charts: ["BarChart"],
    components: ["DatasetComponent", "GridComponent", "TooltipComponent"],
  },
});
