export default {
  extends: ["stylelint-config-standard"],
  rules: {
    "selector-class-pattern": null, // Allow any class naming
    "custom-property-pattern": null, // Allow any custom property naming
    "no-descending-specificity": null, // Too strict for component styles
    "property-no-vendor-prefix": null, // Need -webkit-background-clip for Safari
  },
  ignoreFiles: ["node_modules/**", "cmd/kaunta/assets/vendor/**"],
};
