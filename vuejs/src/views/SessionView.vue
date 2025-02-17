<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const route = useRoute()
const router = useRouter()
const store = useAppStore()
const sessionId = route.params.id
const sessionData = ref(null)

onMounted(async () => {
  store.setLoading(true)
  try {
    if (sessionId === 'new') {
      // Handle new session creation
      sessionData.value = { id: 'new', status: 'initializing' }
    } else {
      // Load existing session
      // Add session loading logic here
      sessionData.value = { id: sessionId, status: 'loaded' }
    }
  } catch (error) {
    console.error('Failed to load session:', error)
  } finally {
    store.setLoading(false)
  }
})
</script>

<template>
  <v-container>
    <v-row>
      <v-col cols="12">
        <v-btn
          icon
          class="mb-4"
          @click="router.back()"
        >
          <v-icon>mdi-arrow-left</v-icon>
        </v-btn>

        <h1 class="text-h4 mb-4">
          {{ sessionId === 'new' ? 'New Session' : \`Session \${sessionId}\` }}
        </h1>

        <v-card v-if="store.loading" class="pa-4">
          <v-progress-circular indeterminate></v-progress-circular>
        </v-card>

        <template v-else-if="sessionData">
          <v-card class="mb-4">
            <v-card-title>Session Status</v-card-title>
            <v-card-text>
              <p>Status: {{ sessionData.status }}</p>
            </v-card-text>
          </v-card>

          <v-card v-if="sessionId === 'new'">
            <v-card-title>Start New Session</v-card-title>
            <v-card-text>
              <v-btn
                color="primary"
                @click="console.log('Start new session')"
              >
                Start
              </v-btn>
            </v-card-text>
          </v-card>
        </template>

        <v-card v-else>
          <v-card-text>
            <p>Session not found</p>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template> 