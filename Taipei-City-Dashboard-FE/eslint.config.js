import js from "@eslint/js";
import pluginVue from "eslint-plugin-vue";
import globals from "globals";

export default [
	js.configs.recommended,
	...pluginVue.configs["flat/recommended"],
	{
		files: ["**/*.js", "**/*.vue"],
		languageOptions: {
			ecmaVersion: "latest",
			sourceType: "module",
			globals: {
				...globals.browser,
				...globals.node,
			},
		},
		rules: {
			indent: ["error", "tab"],
			quotes: "off",
			semi: "off",
			"spaced-comment": "off",
			"no-console": ["error", { allow: ["warn", "error"] }],
			"consistent-return": "off",
			"func-names": "off",
			"object-shorthand": "off",
			"no-process-exit": "off",
			"no-param-reassign": "off",
			"no-return-await": "off",
			"no-underscore-dangle": "off",
			"class-methods-use-this": "off",
			"prefer-destructuring": ["error", { object: true, array: false }],
			"no-unused-vars": [
				"error",
				{ argsIgnorePattern: "req|res|next|val|err" },
			],
			"vue/no-setup-props-destructure": "off",
			"vue/prop-name-casing": "off",
			"vue/require-default-prop": "off",
			"vue/require-prop-types": "off",
			"vue/no-template-shadow": "off",
		},
	},
	{
		files: ["**/*.{test,spec}.{js,ts}", "tests/**/*.{js,ts}"],
		languageOptions: {
			globals: {
				...globals.node,
				describe: "readonly",
				it: "readonly",
				test: "readonly",
				expect: "readonly",
				beforeAll: "readonly",
				afterAll: "readonly",
				beforeEach: "readonly",
				afterEach: "readonly",
				vi: "readonly",
			},
		},
	},
	{
		ignores: [
			"**/public/",
			"**/dist/",
			"**/node_modules/",
			"*.json",
			"playwright-report/",
			"test-results/",
			"coverage/",
		],
	},
];
