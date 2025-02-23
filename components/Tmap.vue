<script setup lang="ts">
const config = useRuntimeConfig();

const tiandituApikey = config.public.tiandituApikey;
const tiandituApibase = "https://t0.tianditu.gov.cn";
const tiandituVec = `${tiandituApibase}/vec_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER=vec&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&tk=${tiandituApikey}`;
const tiandituCva = `${tiandituApibase}/cva_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER=cva&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&tk=${tiandituApikey}`;
const tiandituImg = `${tiandituApibase}/img_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER=img&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&tk=${tiandituApikey}`;
const tiandituCia = `${tiandituApibase}/cia_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER=cia&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&tk=${tiandituApikey}`;

const props = defineProps({
  zoom: {
    type: Number,
    default: 16,
  },
  center: {
    type: Array,
    default: () => [39.906217, 116.3912757],
  },
});
</script>

<template>
  <LMap :zoom :center :options="{ attributionControl: false }">
    <LLayerGroup name="天地图矢量" layerType="base">
      <LTileLayer :url="tiandituVec" />
      <LTileLayer :url="tiandituCva" />
    </LLayerGroup>
    <LLayerGroup name="天地图影像" layerType="base">
      <LTileLayer :url="tiandituImg" />
      <LTileLayer :url="tiandituCia" />
    </LLayerGroup>
    <LControlScale :imperial="false" />
    <LControlLayers position="topright" />
    <LControlAttribution
      prefix="天地图 - GS(2024)0568号"
      position="bottomright"
    />
    <slot />
  </LMap>
</template>
