"use client";

import { App, ConfigProvider, type ThemeConfig } from "antd";
import zhCN from "antd/locale/zh_CN";

const theme: ThemeConfig = {
  token: {
    colorPrimary: "#5e6ad2",
    colorInfo: "#5e6ad2",
    colorSuccess: "#138a45",
    colorWarning: "#b7791f",
    colorError: "#c93535",
    colorBgLayout: "#f6f7f9",
    colorText: "#15171c",
    colorTextSecondary: "#5d6572",
    colorBorder: "#dfe3ea",
    colorBorderSecondary: "#e8eaf0",
    borderRadius: 7,
    borderRadiusLG: 8,
    controlHeight: 34,
    fontFamily: "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    fontSize: 14,
  },
  components: {
    Button: {
      fontWeight: 500,
    },
    Card: {
      bodyPadding: 16,
      headerHeight: 44,
      headerFontSize: 14,
    },
    Layout: {
      bodyBg: "#f6f7f9",
      headerBg: "rgba(255, 255, 255, 0.92)",
      siderBg: "#111317",
    },
    Menu: {
      darkItemBg: "#111317",
      darkItemColor: "#9da4b0",
      darkItemHoverBg: "rgba(255, 255, 255, 0.07)",
      darkItemSelectedBg: "rgba(94, 106, 210, 0.28)",
      darkItemSelectedColor: "#ffffff",
      itemBorderRadius: 6,
    },
    Modal: {
      titleFontSize: 16,
    },
    Table: {
      headerBg: "#f8f9fb",
      headerColor: "#5d6572",
      headerBorderRadius: 8,
      cellPaddingBlock: 11,
      cellPaddingInline: 14,
    },
  },
};

export function AppProvider({ children }: { children: React.ReactNode }) {
  return (
    <ConfigProvider componentSize="middle" locale={zhCN} theme={theme}>
      <App>{children}</App>
    </ConfigProvider>
  );
}
