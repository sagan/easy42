/**
 * PWA & Service Worker utilities for easy42.
 */

export interface FlushResult {
  uninstalled: number;
  cachesDeleted: number;
  success: boolean;
  error?: string;
}

/**
 * Flushes all CacheStorage caches and unregisters all active Service Workers.
 */
export async function flushCacheAndServiceWorker(): Promise<FlushResult> {
  let uninstalled = 0;
  let cachesDeleted = 0;

  try {
    // 1. Unregister all service workers
    if ("serviceWorker" in navigator) {
      const registrations = await navigator.serviceWorker.getRegistrations();
      for (const registration of registrations) {
        const ok = await registration.unregister();
        if (ok) uninstalled++;
      }
    }

    // 2. Clear all cache storages
    if ("caches" in window) {
      const keys = await window.caches.keys();
      for (const key of keys) {
        const ok = await window.caches.delete(key);
        if (ok) cachesDeleted++;
      }
    }

    return {
      uninstalled,
      cachesDeleted,
      success: true,
    };
  } catch (err: unknown) {
    const error = err instanceof Error ? err.message : String(err);
    console.error("Failed to flush cache & service worker:", error);
    return {
      uninstalled,
      cachesDeleted,
      success: false,
      error,
    };
  }
}
