module.exports = {
  cacheId: 'midiconverser',
  cleanupOutdatedCaches: true,
  globDirectory: '.',
  globPatterns: [
    'ebitenui_player.html',
    'ebitenui_player.wasm',
    'wasm_exec.js'
  ],
  swDest: 'ebitenui_player.service-worker.js',
  maximumFileSizeToCacheInBytes: 1073741824,
  inlineWorkboxRuntime: true,
  sourcemap: false
};
