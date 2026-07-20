"use client";

import { Flex, Typography } from "antd";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <Flex align="flex-end" className="page-header" gap={16} justify="space-between" wrap>
      <div>
        <Typography.Title level={2}>{title}</Typography.Title>
        {description ? <Typography.Text type="secondary">{description}</Typography.Text> : null}
      </div>
      {actions ? <Flex gap={8} wrap>{actions}</Flex> : null}
    </Flex>
  );
}
