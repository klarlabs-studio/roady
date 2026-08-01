<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';

const lines = [
  { type: 'command', text: '$ roady init my-project --template web-api' },
  { type: 'success', text: '✓ Initialized my-project with web-api template' },
  { type: 'output', text: '' },
  { type: 'command', text: '$ roady spec review' },
  { type: 'output', text: 'Score: 85/100' },
  { type: 'highlight', text: '2 findings: 1 warning, 1 info' },
  { type: 'output', text: '' },
  { type: 'command', text: '$ roady report --format html -o status.html' },
  { type: 'success', text: '✓ 8 tasks generated from codebase analysis' },
  { type: 'output', text: '' },
  { type: 'command', text: '$ roady task assign auth-api alice' },
  { type: 'success', text: '✓ Assigned auth-api to alice' },
  { type: 'output', text: '' },
  { type: 'command', text: '$ roady query "what tasks are blocked?"' },
  { type: 'output', text: '2 tasks blocked: db-migration, cache-layer' },
  { type: 'dim', text: '  Waiting on: auth-api (in_progress)' },
];

const visibleLines = ref<typeof lines>([]);
const currentIndex = ref(0);

let interval: number | null = null;

onMounted(() => {
  interval = window.setInterval(() => {
    if (currentIndex.value < lines.length) {
      visibleLines.value.push(lines[currentIndex.value]);
      currentIndex.value++;
    } else {
      // Reset and loop
      setTimeout(() => {
        visibleLines.value = [];
        currentIndex.value = 0;
      }, 3000);
    }
  }, 400);
});

onUnmounted(() => {
  if (interval) {
    clearInterval(interval);
  }
});

function getLineClass(type: string): string {
  switch (type) {
    case 'command':
      return 'text-gray-500';
    case 'highlight':
      return 'text-emerald-400';
    case 'success':
      return 'text-emerald-400';
    case 'task':
      return 'text-gray-400';
    case 'dim':
      return 'text-gray-600';
    default:
      return 'text-white';
  }
}
</script>

<template>
  <section id="automation" class="py-24">
    <div class="max-w-7xl mx-auto px-6 flex flex-col lg:flex-row items-center space-y-12 lg:space-y-0 lg:space-x-20">
      <div class="lg:w-1/2">
        <h2 class="text-4xl font-bold text-white mb-6">Built for Continuous Planning</h2>
        <div class="space-y-8 text-gray-400">
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="eye" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">roady watch</h4>
              <p class="text-sm">
                A background sentinel that reacts to document changes, instantly updating your specification and checking for plan drift.
              </p>
            </div>
          </div>
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="git-commit" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">Git Sync Loop</h4>
              <p class="text-sm">
                Roady parses your commit history for [roady:task-id] markers to automate status transitions.
              </p>
            </div>
          </div>
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="shield-check" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">No model of its own</h4>
              <p class="text-sm">
                Roady runs no inference and holds no API key. It assembles the context a model needs and hands it to the agent that already has one.
              </p>
            </div>
          </div>
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="link" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">Plugin Sync</h4>
              <p class="text-sm">
                Sync tasks with external systems via plugins (GitHub Issues, Jira, Linear).
              </p>
            </div>
          </div>
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="users" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">Team Collaboration</h4>
              <p class="text-sm">
                Role-based access, task assignment, optimistic locking, and git-based workspace sync for multi-user workflows.
              </p>
            </div>
          </div>
          <div class="flex items-start space-x-4">
            <div class="w-8 h-8 bg-violet-500/20 rounded-lg flex-shrink-0 flex items-center justify-center text-violet-400 mt-1">
              <i data-lucide="package" class="w-5 h-5"></i>
            </div>
            <div>
              <h4 class="text-white font-semibold mb-1">Go SDK</h4>
              <p class="text-sm">
                Public Go client package with typed helpers, versioned MCP schema, and OpenAPI spec generation.
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Animated Terminal -->
      <div class="lg:w-1/2 glass rounded-3xl p-2 shadow-2xl overflow-hidden">
        <div class="bg-[#0B0E14] rounded-2xl p-6 mono text-xs leading-relaxed min-h-[320px]">
          <div class="flex space-x-2 mb-4">
            <div class="w-3 h-3 bg-red-500/50 rounded-full"></div>
            <div class="w-3 h-3 bg-yellow-500/50 rounded-full"></div>
            <div class="w-3 h-3 bg-green-500/50 rounded-full"></div>
          </div>
          <div class="space-y-1">
            <div
              v-for="(line, index) in visibleLines"
              :key="index"
              :class="getLineClass(line.type)"
            >
              {{ line.text }}
            </div>
            <span class="inline-block w-2 h-4 bg-white/50 animate-pulse"></span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
