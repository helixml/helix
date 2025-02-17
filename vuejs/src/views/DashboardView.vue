<script setup lang="ts">
import { onMounted } from 'vue'
import { useAppStore } from '@/stores/app'

const store = useAppStore()

onMounted(() => {
  store.setLoading(true)
  // Add initialization logic here
  setTimeout(() => {
    store.setLoading(false)
  }, 1000)
})
</script>

<template>
  <v-container>
    <v-row>
      <v-col cols="12">
        <h1 class="text-h4 mb-4">Dashboard</h1>
        
        <v-card v-if="store.loading" class="pa-4">
          <v-progress-circular indeterminate></v-progress-circular>
        </v-card>
        
        <template v-else>
          <v-card class="mb-4">
            <v-card-title>Quick Actions</v-card-title>
            <v-card-text>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-btn
                    block
                    color="primary"
                    to="/session/new"
                  >
                    New Session
                  </v-btn>
                </v-col>
              </v-row>
            </v-card-text>
          </v-card>
          
          <v-card>
            <v-card-title>Recent Sessions</v-card-title>
            <v-card-text>
              <p v-if="!store.isAuthenticated">Please log in to view your sessions</p>
              <p v-else>No recent sessions found</p>
            </v-card-text>
          </v-card>
        </template>
      </v-col>
    </v-row>
  </v-container>
</template> 