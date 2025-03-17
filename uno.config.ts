import {
  defineConfig,
  presetWind3,
  presetIcons,
  presetTypography,
} from "unocss";

export default defineConfig({
  presets: [presetWind3(), presetIcons(), presetTypography()],
});
