import type { Metadata, Viewport } from "next";
import { Footer, Layout, Navbar } from "nextra-theme-docs";
import { Head } from "nextra/components";
import { getPageMap } from "nextra/page-map";
import "nextra-theme-docs/style.css";
import "./globals.css";

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
  userScalable: true,
  themeColor: "#0a0b14",
};

export const metadata: Metadata = {
  title: "kas docs",
  description:
    "Documentation for kasmos — a TUI-based orchestration platform for managing AI agents, wave-based tasks, headless execution, daemon workflows, and the kas CLI.",
  keywords: [
    "kasmos",
    "kas",
    "tui",
    "docs",
    "agent",
    "orchestration",
    "daemon",
    "cli",
    "terminal",
  ],
  authors: [{ name: "kastheco" }],
  openGraph: {
    title: "kas docs",
    description:
      "Documentation for kasmos — TUI orchestration, headless execution, wave-based workflows, and CLI reference",
    url: "https://github.com/kastheco/kasmos",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "kas docs",
    description:
      "Documentation for kasmos — TUI orchestration, headless execution, wave-based workflows, and CLI reference",
  },
};

const navbar = (
  <Navbar
    logo={<b>kas docs</b>}
    projectLink="https://github.com/kastheco/kasmos"
  />
);

const footer = (
  <Footer>MIT {new Date().getFullYear()} © kastheco.</Footer>
);

export default async function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head
        color={{
          hue: 28,
          saturation: 82,
          lightness: { dark: 67, light: 40 },
        }}
      />
      <body>
        <Layout
          navbar={navbar}
          pageMap={await getPageMap()}
          docsRepositoryBase="https://github.com/kastheco/kasmos/blob/main/web/docs/src/content"
          footer={footer}
        >
          {children}
        </Layout>
      </body>
    </html>
  );
}
