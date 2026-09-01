<!--
  PublicFormResponderView — wraps CollabFormResponder for the /form/:token
  public route. No auth required.
-->
<template>
  <div class="public-form-view">
    <CollabFormResponder :token="token" />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import CollabFormResponder from "@/components/collab/CollabFormResponder.vue";

const route = useRoute();
const token = computed(() => String(route.params.token || ""));
</script>

<style scoped>
.public-form-view {
  min-height: 100vh;
  background: #f6f8fa;
}
</style>
