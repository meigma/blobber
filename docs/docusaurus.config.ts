import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "Blobber",
  tagline: "Push and pull files to OCI container registries",
  favicon: "img/logo-icon.png",

  future: {
    v4: true,
  },

  url: "https://blobber.meigma.dev",
  baseUrl: "/",

  organizationName: "meigma",
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
          editUrl: "https://github.com/meigma/blobber/edit/master/docs/",
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
      logo: {
        alt: "Blobber Logo",
        src: "img/logo-icon-black.png",
        srcDark: "img/logo-icon.png",
      },
      items: [
        {
          type: "docSidebar",
          sidebarId: "docs",
          position: "left",
          label: "Documentation",
          to: "/docs/intro",
        },
        {
          href: "https://github.com/meigma/blobber",
          label: "GitHub",
          position: "right",
          className: "navbar__item--github",
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
              href: "https://github.com/meigma/blobber",
            },
            {
              label: "Go Reference",
              href: "https://pkg.go.dev/github.com/meigma/blobber",
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Meigma. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ["bash", "go", "json", "yaml"],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
