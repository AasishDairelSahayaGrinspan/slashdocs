# RazorSense

A copy of the RazorSense component from the Blade
design system (`packages/blade/src/components/Spark` in
https://github.com/razorpay/blade, `@razorpay/blade@12.128.0`, MIT license in
`LICENSE`). We copy them so that the landing page can use them without Blade's
provider, styled-components, and React Native dependencies.

Local changes:

- The color scheme comes from `useResolvedTheme`, not from Blade's provider.
- `useMergeRefs` and `useId` are local or React versions.
- Assets load from `/razorsense` on our own origin, not from a CDN.
  `frontend/public/razorsense` holds only the files of the default preset
  in both themes. Copy more from `@razorpay/blade/assets/spark` to use
  another preset.
