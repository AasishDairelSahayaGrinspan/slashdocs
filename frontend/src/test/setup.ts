import "@testing-library/jest-dom/vitest";

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

Object.defineProperty(globalThis, "ResizeObserver", {
  value: ResizeObserverStub,
  writable: true,
});

Object.defineProperty(Element.prototype, "hasPointerCapture", {
  value: () => false,
});
Object.defineProperty(Element.prototype, "setPointerCapture", {
  value: () => undefined,
});
Object.defineProperty(Element.prototype, "releasePointerCapture", {
  value: () => undefined,
});
