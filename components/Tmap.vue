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
  clickable: {
    type: Boolean,
    default: false,
  },
  tempMarker: {
    type: Array,
    default: null,
  },
});

const emit = defineEmits(['map-click']);

// 处理地图点击事件
const handleMapClick = (event) => {
  if (props.clickable) {
    emit('map-click', {
      latlng: {
        lat: event.latlng.lat,
        lng: event.latlng.lng,
      },
    });
  }
};

// 添加调试信息
watch(() => props.tempMarker, (newMarker) => {
  if (newMarker) {
    console.log('临时标记位置:', newMarker);
  }
});
</script>

<template>
  <ClientOnly>
    <div class="w-full h-full">
      <LMap
        :zoom
        :center
        :options="{ attributionControl: false }"
        :use-global-leaflet="false"
        @click="handleMapClick"
      >
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
        
        <!-- 临时标记（使用默认 Leaflet 标记） -->
        <LMarker 
          v-if="tempMarker" 
          :lat-lng="tempMarker"
        />
        
        <slot />
      </LMap>
    </div>
  </ClientOnly>
</template>

<style>
/* 移除所有自定义样式 */
</style>
