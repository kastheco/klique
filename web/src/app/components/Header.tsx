"use client";

import { useEffect, useState } from "react";
import styles from "./Header.module.css";
import PixelBee from "./PixelBee";

export default function Header() {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 50);
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header className={`${styles.header} ${scrolled ? styles.scrolled : ""}`}>
      <a href="#" className={styles.logoWrapper}>
        <PixelBee scale={2} bob={false} />
        <span className={styles.logo}>kas</span>
      </a>
      <nav className={styles.nav}>
        <a
          href="https://github.com/kastheco/kasmos"
          target="_blank"
          rel="noopener noreferrer"
          className={styles.navLink}
        >
          GitHub
        </a>
        <a
          href={
            process.env.NODE_ENV === "production" ? "/kasmos/docs" : "/docs"
          }
          className={styles.navLink}
        >
          Docs
        </a>
        <a
          href="https://github.com/kastheco/kasmos/releases"
          target="_blank"
          rel="noopener noreferrer"
          className={styles.navLink}
        >
          Releases
        </a>
      </nav>
    </header>
  );
}
