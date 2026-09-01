import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Modbus Console",
  description: "Web Modbus Poll + Slave console powered by a local Core Engine",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
