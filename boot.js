"use strict";

// Запускает Go WebAssembly. Логика страницы собрана в motion.wasm.
const go = new Go();

async function startMotion() {
  const response = await fetch("/static/motion.wasm");
  if (!response.ok) {
    throw new Error("motion.wasm: " + response.status);
  }
  let result;
  if (WebAssembly.instantiateStreaming) {
    try {
      result = await WebAssembly.instantiateStreaming(response, go.importObject);
    } catch (err) {
      const again = await fetch("/static/motion.wasm");
      if (!again.ok) {
        throw err;
      }
      const bytes = await again.arrayBuffer();
      result = await WebAssembly.instantiate(bytes, go.importObject);
    }
  } else {
    const bytes = await response.arrayBuffer();
    result = await WebAssembly.instantiate(bytes, go.importObject);
  }
  await go.run(result.instance);
}

startMotion().catch((err) => {
  console.error(err);
});
