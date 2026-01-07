import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "Blobber",
  tagline: "Push and pull files to OCI container registries",
  favicon: "img/favicon.ico",

  future: {
    v4: true,
  },

  url: "https://blobber.gilman.io",
  baseUrl: "/",

  organizationName: "gilmanlab",
  projectName: "blobber",

  onBrokenLinks: "throw",
  onBrokenMarkdownLinks: "warn",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          routeBasePath: "/",
          editUrl: "https://github.com/gilmanlab/blobber/edit/master/docs/",
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: "dark",
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: "Blobber",
      items: [
        {
          type: "docSidebar",
          sidebarId: "docs",
          position: "left",
          label: "Documentation",
        },
        {
          href: "https://pkg.go.dev/github.com/gilmanlab/blobber",
          label: "Go Reference",
          position: "right",
        },
        {
          href: "https://github.com/gilmanlab/blobber",
          label: "GitHub",
          position: "right",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Resources",
          items: [
            {
              label: "GitHub",
              href: "https://github.com/gilmanlab/blobber",
            },
            {
              label: "Go Reference",
              href: "https://pkg.go.dev/github.com/gilmanlab/blobber",
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Gilman Lab. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ["bash", "go", "json", "yaml"],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
