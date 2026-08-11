import React, { FC, useInsertionEffect, useMemo } from "react";
import { createFileTreeIconResolver, getBuiltInSpriteSheet } from "@pierre/trees";

const ICON_SPRITE_ID = "helix-pierre-file-icon-sprite";

const CUSTOM_ICON_SPRITE = `
<svg xmlns="http://www.w3.org/2000/svg" width="0" height="0" aria-hidden="true">
  <symbol id="helix-file-icon-package-json" viewBox="0 0 32 32">
    <path d="M2 2H30V30H2" fill="#c12127" />
    <path d="M7.25 7.25h17.5v17.5h-3.5v-14H16v14H7.25" fill="#fff" />
  </symbol>
</svg>`;

const ICON_COLORS: Record<string, readonly [light: string, dark: string]> = {
  astro: ["#a631be", "#d568ea"],
  babel: ["#b48f00", "#ffd452"],
  bash: ["#199f43", "#5ecc71"],
  biome: ["#1a85d4", "#69b1ff"],
  bootstrap: ["#693acf", "#9d6afb"],
  browserslist: ["#b48f00", "#ffd452"],
  bun: ["#594c5b", "#79697b"],
  c: ["#1a85d4", "#69b1ff"],
  claude: ["#d47628", "#ffa359"],
  cpp: ["#1a85d4", "#69b1ff"],
  css: ["#693acf", "#9d6afb"],
  database: ["#a631be", "#d568ea"],
  default: ["#84848a", "#adadb1"],
  docker: ["#1a85d4", "#69b1ff"],
  eslint: ["#693acf", "#9d6afb"],
  font: ["#84848a", "#adadb1"],
  git: ["#d5512f", "#ff8c5b"],
  go: ["#1ca1c7", "#68cdf2"],
  graphql: ["#d32a61", "#ff678d"],
  html: ["#d47628", "#ffa359"],
  image: ["#d32a61", "#ff678d"],
  javascript: ["#b48f00", "#ffd452"],
  json: ["#d47628", "#ffa359"],
  markdown: ["#199f43", "#5ecc71"],
  mcp: ["#17a5af", "#64d1db"],
  nextjs: ["#84848a", "#adadb1"],
  npm: ["#d52c36", "#ff6762"],
  oxc: ["#1ca1c7", "#68cdf2"],
  postcss: ["#d52c36", "#ff6762"],
  prettier: ["#17a5af", "#64d1db"],
  python: ["#1a85d4", "#69b1ff"],
  react: ["#1ca1c7", "#68cdf2"],
  ruby: ["#d52c36", "#ff6762"],
  rust: ["#d47628", "#ffa359"],
  sass: ["#d32a61", "#ff678d"],
  stylelint: ["#84848a", "#adadb1"],
  svelte: ["#d52c36", "#ff6762"],
  svg: ["#d47628", "#ffa359"],
  svgo: ["#199f43", "#5ecc71"],
  swift: ["#d47628", "#ffa359"],
  table: ["#17a5af", "#64d1db"],
  tailwind: ["#1ca1c7", "#68cdf2"],
  terraform: ["#693acf", "#9d6afb"],
  text: ["#84848a", "#adadb1"],
  typescript: ["#1a85d4", "#69b1ff"],
  vite: ["#a631be", "#d568ea"],
  vscode: ["#1a85d4", "#69b1ff"],
  vue: ["#199f43", "#5ecc71"],
  wasm: ["#693acf", "#9d6afb"],
  webpack: ["#1a85d4", "#69b1ff"],
  yml: ["#d52c36", "#ff6762"],
  zig: ["#d47628", "#ffa359"],
  zip: ["#d47628", "#ffa359"],
};

const iconResolver = createFileTreeIconResolver({
  set: "complete",
  colored: true,
  spriteSheet: CUSTOM_ICON_SPRITE,
  byFileName: { "package.json": "helix-file-icon-package-json" },
});

export function ensureChangedFileIconSprite(): void {
  if (typeof document === "undefined" || document.getElementById(ICON_SPRITE_ID)) return;
  const container = document.createElement("div");
  container.id = ICON_SPRITE_ID;
  container.setAttribute("aria-hidden", "true");
  container.style.position = "absolute";
  container.style.width = "0";
  container.style.height = "0";
  container.style.overflow = "hidden";
  container.style.pointerEvents = "none";
  container.innerHTML = `${getBuiltInSpriteSheet("complete")}${CUSTOM_ICON_SPRITE}`;
  document.body.prepend(container);
}

export function resolveChangedFileIcon(path: string, darkMode: boolean) {
  const icon = iconResolver.resolveIcon("file-tree-icon-file", path);
  const colors = ICON_COLORS[icon.token || "default"] || ICON_COLORS.default;
  return {
    name: icon.name,
    token: icon.token,
    viewBox: icon.viewBox || "0 0 16 16",
    color: colors[darkMode ? 1 : 0],
  };
}

interface ChangedFileIconProps {
  path: string;
  darkMode: boolean;
  size?: number;
}

const ChangedFileIcon: FC<ChangedFileIconProps> = ({ path, darkMode, size = 14 }) => {
  useInsertionEffect(ensureChangedFileIconSprite, []);
  const icon = useMemo(
    () => resolveChangedFileIcon(path, darkMode),
    [darkMode, path],
  );

  return (
    <svg
      aria-hidden="true"
      data-pierre-icon={icon.name}
      data-icon-token={icon.token}
      viewBox={icon.viewBox}
      width={size}
      height={size}
      style={{ color: icon.color, flexShrink: 0 }}
    >
      <use href={`#${icon.name}`} />
    </svg>
  );
};

export default ChangedFileIcon;
