import eslint from "@eslint/js";
import globals from "globals";
import accessibility from "eslint-plugin-jsx-a11y";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";

export default [{
    ignores: ["dist/**"],
}, eslint.configs.recommended, {
    files: ["**/*.{js,jsx}"],
    languageOptions: {
        ecmaVersion: "latest",
        globals: globals.browser,
        parserOptions: {
            ecmaFeatures: {
                jsx: true
            },
            sourceType: "module"
        }
    },
    plugins: {
        "jsx-a11y": accessibility,
        react,
        "react-hooks": reactHooks
    },
    rules: {
        ...accessibility.flatConfigs.recommended.rules,
        "no-unused-vars": ["error", { argsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" }],
        "react/jsx-uses-vars": "error",
        "react-hooks/exhaustive-deps": "error",
        "react-hooks/rules-of-hooks": "error"
    },
    settings: {
        "jsx-a11y": {
            polymorphicPropName: "as"
        }
    }
}];
