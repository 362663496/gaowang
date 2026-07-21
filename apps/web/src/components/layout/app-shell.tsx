"use client";

import {
  AppstoreOutlined,
  AuditOutlined,
  BarChartOutlined,
  DatabaseOutlined,
  DashboardOutlined,
  KeyOutlined,
  LogoutOutlined,
  MenuOutlined,
  SettingOutlined,
  ShopOutlined,
  ShoppingOutlined,
  SwapOutlined,
  TeamOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Button, Drawer, Flex, Layout, Menu, Result, Spin, Tag, Tooltip, Typography } from "antd";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { PermissionGate } from "@/components/layout/permission-gate";
import { SessionProvider, useSession } from "@/components/layout/session-context";
import { logout } from "@/lib/api";
import { anyPermission, hasPermission } from "@/lib/permissions";

const { Content, Header, Sider } = Layout;

type NavItem = {
  label: string;
  href: string;
  icon: React.ReactNode;
  /** Page entry permission(s). Settings is always visible. */
  permissions?: string[];
  anyOf?: string[];
};

const navItems: NavItem[] = [
  { label: "仪表盘", href: "/dashboard", icon: <DashboardOutlined /> },
  { label: "商品", href: "/products", icon: <ShoppingOutlined />, permissions: ["product.read"] },
  { label: "店铺", href: "/shops", icon: <ShopOutlined />, permissions: ["shop.read"] },
  { label: "当前库存", href: "/inventory", icon: <AppstoreOutlined />, permissions: ["inventory.read"] },
  { label: "流水记录", href: "/stock-movements", icon: <SwapOutlined />, permissions: ["movement.read"] },
  {
    label: "报表",
    href: "/reports",
    icon: <BarChartOutlined />,
    anyOf: ["report.sales_summary", "report.sales_trend", "report.product_ranking", "report.shop_ranking"],
  },
  { label: "操作记录", href: "/audit-logs", icon: <AuditOutlined />, permissions: ["audit.read"] },
  { label: "用户管理", href: "/users", icon: <TeamOutlined />, permissions: ["user.read"] },
  { label: "权限管理", href: "/permissions", icon: <KeyOutlined />, permissions: ["permission.read"] },
  { label: "备份", href: "/settings/backups", icon: <DatabaseOutlined />, permissions: ["backup.read"] },
  { label: "设置", href: "/settings", icon: <SettingOutlined /> },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <AppShellInner>{children}</AppShellInner>
    </SessionProvider>
  );
}

function AppShellInner({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { user, permissions, loading, clear } = useSession();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  const visibleItems = useMemo(
    () =>
      navItems.filter((item) => {
        if (item.anyOf) return anyPermission(permissions, item.anyOf);
        if (item.permissions) return item.permissions.every((key) => hasPermission(permissions, key));
        return true;
      }),
    [permissions],
  );
  const active = visibleItems.find(
    (item) => pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(`${item.href}/`)),
  );
  const activeDefinition = navItems.find(
    (item) => pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(`${item.href}/`)),
  );

  if (loading) {
    return (
      <Flex align="center" className="session-loading" justify="center">
        <Spin tip="正在验证登录状态" />
      </Flex>
    );
  }

  if (!user) {
    return (
      <Flex align="center" className="session-loading" justify="center">
        <Spin tip="正在跳转登录" />
      </Flex>
    );
  }

  async function onLogout() {
    setLoggingOut(true);
    try {
      await logout();
    } catch {
      // Still clear local session state if the network call fails.
    } finally {
      clear();
      router.replace("/login");
      setLoggingOut(false);
    }
  }

  const menu = (
    <Menu
      items={visibleItems.map((item) => ({
        key: item.href,
        icon: item.icon,
        label: (
          <Link href={item.href} onClick={() => setMobileOpen(false)}>
            {item.label}
          </Link>
        ),
      }))}
      mode="inline"
      selectedKeys={active ? [active.href] : []}
      theme="dark"
    />
  );

  const pageContent = activeDefinition ? (
    <PermissionGate anyOf={activeDefinition.anyOf} permission={activeDefinition.permissions?.[0]}>
      {children}
    </PermissionGate>
  ) : (
    children
  );

  // Dashboard / settings have no single permission; PermissionGate with neither always allows.
  // For multi-permission pages use anyOf. For single permission pages use first permission key.
  // Settings has neither — always allowed.

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
            <Typography.Text strong>{active?.label ?? activeDefinition?.label ?? "库存后台"}</Typography.Text>
          </Flex>
          <Flex align="center" gap={10}>
            <Tag className="session-tag" color="green" icon={<UserOutlined />}>
              {user.role} · {user.name}
            </Tag>
            <Tooltip title="退出登录">
              <Button aria-label="退出登录" icon={<LogoutOutlined />} loading={loggingOut} type="text" onClick={() => void onLogout()} />
            </Tooltip>
          </Flex>
        </Header>
        <Content className="app-content">
          {permissions.length === 0 && pathname === "/dashboard" ? (
            <Result status="info" subTitle="请联系管理员为你的 staff 账号分配业务权限。" title="当前账号暂无业务权限" />
          ) : (
            pageContent
          )}
        </Content>
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
      <div className="brand-mark">
        <ShoppingOutlined />
      </div>
      <div>
        <div className="brand-name">Gaowang</div>
        {!compact ? <div className="brand-caption">Inventory Command</div> : null}
      </div>
    </div>
  );
}
