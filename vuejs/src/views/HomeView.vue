<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const router = useRouter()
const store = useAppStore()

const handleGetStarted = () => {
  store.setAuthenticated(true)
  router.push('/dashboard')
}
</script>

<template>
  <v-container>
    <v-row class="text-center">
      <v-col cols="12">
        <h1 class="text-h2 font-weight-bold mb-4">
          Welcome to Helix
        </h1>
        <p class="text-h5 mb-6">
          Your AI Assistant Platform
        </p>
        <v-btn
          color="primary"
          size="large"
          @click="handleGetStarted"
          v-if="!store.isAuthenticated"
        >
          Get Started
        </v-btn>
      </v-col>
    </v-row>
  </v-container>
</template>
