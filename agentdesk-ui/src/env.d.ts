// Vite+'s TypeScript checker does not parse Vue SFCs; vue-tsc checks them in build.
declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent;
  export default component;
}
