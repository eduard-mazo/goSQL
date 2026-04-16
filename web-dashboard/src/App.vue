<script setup lang="ts">
import AppTopbar from '@/components/layout/AppTopbar.vue'
import AppSidebar from '@/components/layout/AppSidebar.vue'
</script>

<template>
  <div class="min-h-screen bg-epm-gray-50">
    <AppTopbar />
    <div class="flex">
      <AppSidebar />
      <main class="flex-1 overflow-y-auto min-h-[calc(100vh-56px)]">
        <router-view />
      </main>
    </div>
  </div>
</template>
