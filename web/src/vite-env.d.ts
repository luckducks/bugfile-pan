/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_UPLOAD_BASE?: string;
  readonly VITE_SITE_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
