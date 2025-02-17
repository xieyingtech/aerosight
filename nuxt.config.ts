// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: "2024-11-01",
  modules: ["nuxt-auth-utils", "@prisma/nuxt", "@nuxt/ui"],
  app: {
    head: {
      title: "协盈穹目",
    },
  },
  prisma: {
    installStudio: false,
  },
  vite: {
    resolve: {
      alias: {
        ".prisma/client/index-browser":
          "./node_modules/.prisma/client/index-browser.js",
      },
    },
  },
});
