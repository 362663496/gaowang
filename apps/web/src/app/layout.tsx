import type { Metadata } from "next";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { AppProvider } from "@/components/layout/app-provider";
import "@/styles/globals.css";

export const metadata: Metadata = {
  title: "Gaowang 库存后台",
  description: "轻量库存与销售出库后台",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>
        <AntdRegistry>
          <AppProvider>{children}</AppProvider>
        </AntdRegistry>
      </body>
    </html>
  );
}
