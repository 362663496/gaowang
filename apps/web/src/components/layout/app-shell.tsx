"use client";

import {
  AppstoreOutlined,
  AuditOutlined,
  BarChartOutlined,
  DatabaseOutlined,
  DashboardOutlined,
  LogoutOutlined,
  MenuOutlined,
  SettingOutlined,
  ShopOutlined,
  ShoppingOutlined,
  SwapOutlined,
  TeamOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Button, Drawer, Flex, Layout, Menu, Spin, Tag, Tooltip, Typography } from "antd";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { apiDeleteSession, devSessionEvent, readDevSession, type DevSession, type Role } from "@/lib/api";

const { Content, Header, Sider } = Layout;

const navItems = [
  { label: "仪表盘", href: "/dashboard", icon: <DashboardOutlined /> },
  { label: "商品", href: "/products", icon: <ShoppingOutlined /> },
  { label: "店铺", href: "/shops", icon: <ShopOutlined /> },
  { label: "当前库存", href: "/inventory", icon: <AppstoreOutlined /> },
  { label: "流水记录", href: "/stock-movements", icon: <SwapOutlined /> },
  { label: "报表", href: "/reports", icon: <BarChartOutlined /> },
  { label: "操作记录", href: "/audit-logs", icon: <AuditOutlined />, roles: ["admin"] as Role[] },
  { label: "用户管理", href: "/users", icon: <TeamOutlined />, roles: ["admin"] as Role[] },
  { label: "备份", href: "/settings/backups", icon: <DatabaseOutlined /> },
  { label: "设置", href: "/settings", icon: <SettingOutlined /> },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [session, setSession] = useState<DevSession | null>(null);

  useEffect(() => {
    const update = () => setSession(readDevSession());
    window.addEventListener(devSessionEvent, update);
    window.addEventListener("storage", update);
    update();
    return () => {
      window.removeEventListener(devSessionEvent, update);
      window.removeEventListener("storage", update);
    };
  }, []);

  useEffect(() => {
    if (session && !session.userId) router.replace("/login");
  }, [router, session]);

  const visibleItems = useMemo(
    () => navItems.filter((item) => !item.roles || (session && item.roles.includes(session.role))),
    [session],
  );
  const active = visibleItems.find((item) => pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(`${item.href}/`)));

  if (!session?.userId) {
    return (
      <Flex align="center" className="session-loading" justify="center">
        <Spin tip="正在验证登录状态" />
      </Flex>
    );
  }

  function logout() {
    apiDeleteSession();
    router.replace("/login");
  }

  const menu = (
    <Menu
      items={visibleItems.map((item) => ({
        key: item.href,
        icon: item.icon,
        label: <Link href={item.href} onClick={() => setMobileOpen(false)}>{item.label}</Link>,
      }))}
      mode="inline"
      selectedKeys={active ? [active.href] : []}
      theme="dark"
    />
  );

  return (
    <Layout className="app-shell">
      <Sider className="app-sider" theme="dark" width={232}>
        <Brand />
        {menu}
      </Sider>

      <Layout className="app-main-layout">
        <Header className="app-header">
          <Flex align="center" gap={12}>
            <Button
              aria-label="打开导航"
              className="mobile-menu-button"
              icon={<MenuOutlined />}
              type="text"
              onClick={() => setMobileOpen(true)}
            />
            <Typography.Text strong>{active?.label ?? "库存后台"}</Typography.Text>
          </Flex>
          <Flex align="center" gap={10}>
            <Tag className="session-tag" color="green" icon={<UserOutlined />}>
              {session.role} · {session.userId.slice(0, 8)}
            </Tag>
            <Tooltip title="退出登录">
              <Button aria-label="退出登录" icon={<LogoutOutlined />} type="text" onClick={logout} />
            </Tooltip>
          </Flex>
        </Header>
        <Content className="app-content">{children}</Content>
      </Layout>

      <Drawer
        className="mobile-navigation"
        open={mobileOpen}
        placement="left"
        title={<Brand compact />}
        width={280}
        onClose={() => setMobileOpen(false)}
      >
        {menu}
      </Drawer>
    </Layout>
  );
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={compact ? "brand brand-compact" : "brand"}>
      <div className="brand-mark"><ShoppingOutlined /></div>
      <div>
        <div className="brand-name">Gaowang</div>
        {!compact ? <div className="brand-caption">Inventory Command</div> : null}
      </div>
    </div>
  );
}
