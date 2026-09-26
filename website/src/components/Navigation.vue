<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';

const isOpen = ref(false);
const isScrolled = ref(false);

const navLinks = [
  { href: '#features', label: 'Features' },
  { href: '#commands', label: 'Commands' },
  { href: '#mcp', label: 'MCP' },
  { href: '/roady/roadmap', label: 'Roadmap' },
  { href: '/roady/docs', label: 'Docs' },
];

function handleScroll() {
  isScrolled.value = window.scrollY > 20;
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll);
});

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll);
});
</script>

<template>
  <nav
    class="fixed w-full z-50 transition-all duration-300"
    :class="[
      isScrolled ? 'glass border-b border-white/5' : 'bg-transparent',
    ]"
  >
    <div class="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
      <!-- Logo -->
      <a href="#" class="flex items-center space-x-3">
        <img src="/roady/logo.svg" class="w-8 h-8" alt="Roady Logo">
        <span class="text-white font-bold text-xl tracking-tight">Roady</span>
      </a>

      <!-- Desktop Navigation -->
      <div class="hidden md:flex items-center space-x-8 text-sm font-medium uppercase tracking-wider">
        <a
          v-for="link in navLinks"
          :key="link.href"
          :href="link.href"
          class="text-gray-400 hover:text-violet-400 transition-colors"
        >
          {{ link.label }}
        </a>
        <a
          href="https://github.com/felixgeelhaar/roady"
          target="_blank"
          rel="noopener noreferrer"
          class="text-white bg-violet-600 px-4 py-2 rounded-full hover:bg-violet-500 transition-all"
        >
          GitHub
        </a>
      </div>

      <!-- Mobile Menu Button -->
      <button
        @click="isOpen = !isOpen"
        class="md:hidden p-2 text-gray-400 hover:text-white transition-colors"
        aria-label="Toggle menu"
      >
        <i :data-lucide="isOpen ? 'x' : 'menu'" class="w-6 h-6"></i>
      </button>
    </div>

    <!-- Mobile Menu -->
    <div
      v-show="isOpen"
      class="md:hidden glass border-t border-white/5"
    >
      <div class="px-6 py-4 space-y-4">
        <a
          v-for="link in navLinks"
          :key="link.href"
          :href="link.href"
          class="block text-gray-400 hover:text-violet-400 transition-colors uppercase text-sm tracking-wider"
          @click="isOpen = false"
        >
          {{ link.label }}
        </a>
        <a
          href="https://github.com/felixgeelhaar/roady"
          target="_blank"
          rel="noopener noreferrer"
          class="block text-center text-white bg-violet-600 px-4 py-2 rounded-full hover:bg-violet-500 transition-all"
        >
          GitHub
        </a>
      </div>
    </div>
  </nav>
</template>
