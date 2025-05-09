import { definePreset } from "@primeuix/themes";
import Aura from "@primeuix/themes/aura";

export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: [
    "nuxt-auth-utils",
    "nuxt-echarts",
    "@nuxtjs/leaflet",
    "@unocss/nuxt",
    "@vueuse/nuxt",
    "@primevue/nuxt-module",
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
  primevue: {
    options: {
      theme: {
        preset: definePreset(Aura, {
          semantic: {
            primary: {
              50: "{zinc.50}",
              100: "{zinc.100}",
              200: "{zinc.200}",
              300: "{zinc.300}",
              400: "{zinc.400}",
              500: "{zinc.500}",
              600: "{zinc.600}",
              700: "{zinc.700}",
              800: "{zinc.800}",
              900: "{zinc.900}",
              950: "{zinc.950}",
            },
            colorScheme: {
              light: {
                primary: {
                  color: "{zinc.950}",
                  inverseColor: "#ffffff",
                  hoverColor: "{zinc.900}",
                  activeColor: "{zinc.800}",
                },
                highlight: {
                  background: "{zinc.950}",
                  focusBackground: "{zinc.700}",
                  color: "#ffffff",
                  focusColor: "#ffffff",
                },
              },
              dark: {
                primary: {
                  color: "{zinc.50}",
                  inverseColor: "{zinc.950}",
                  hoverColor: "{zinc.100}",
                  activeColor: "{zinc.200}",
                },
                highlight: {
                  background: "rgba(250, 250, 250, .16)",
                  focusBackground: "rgba(250, 250, 250, .24)",
                  color: "rgba(255,255,255,.87)",
                  focusColor: "rgba(255,255,255,.87)",
                },
              },
            },
          },
        }),
      },
    },
  },
});
