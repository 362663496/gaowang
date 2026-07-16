"use client";

import { ExportOutlined, ImportOutlined, SlidersOutlined } from "@ant-design/icons";
import { Alert, Button, Flex, Form, Input, InputNumber, Modal, Select, Space } from "antd";
import { useMemo, useState } from "react";
import { ProductCombobox } from "@/features/product-combobox";
import type { InventorySnapshot, Product, Shop } from "@/features/types";
import { apiPost } from "@/lib/api";
import { centsToYuanInput, formatQuantity, yuanToCents } from "@/lib/format";

type Props = {
  products: Product[];
  shops: Shop[];
  inventory: InventorySnapshot[];
  onDone: (message: string) => void;
};

type InboundValues = { product_id: string; shop_id?: string; quantity: number; unit_yuan: number };
type OutboundValues = { product_id: string; shop_id: string; quantity: number; sale_yuan: number };
type AdjustmentValues = { product_id: string; quantity_delta: number; reason: string };

export function InventoryActions({ products, shops, inventory, onDone }: Props) {
  return (
    <Space wrap>
      <InboundForm products={products} shops={shops} onDone={onDone} />
      <OutboundForm inventory={inventory} products={products} shops={shops} onDone={onDone} />
      <AdjustmentForm products={products} onDone={onDone} />
    </Space>
  );
}

function InboundForm({ products, shops, onDone }: Pick<Props, "products" | "shops" | "onDone">) {
  const [form] = Form.useForm<InboundValues>();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const selectableProducts = useSelectableProducts(products);

  async function submit(values: InboundValues) {
    setSaving(true);
    setError("");
    try {
      await apiPost("/inventory/inbound", {
        product_id: values.product_id,
        shop_id: values.shop_id ?? "",
        quantity: values.quantity,
        unit_cents: yuanToCents(String(values.unit_yuan ?? 0)),
      });
      setOpen(false);
      onDone("入库已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "入库失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Button icon={<ImportOutlined />} type="primary" onClick={() => setOpen(true)}>入库</Button>
      <Modal
        destroyOnHidden
        footer={null}
        open={open}
        title="新建入库"
        onCancel={() => setOpen(false)}
        afterClose={() => { form.resetFields(); setError(""); }}
      >
        <Form<InboundValues>
          form={form}
          initialValues={{ quantity: 1, unit_yuan: 0 }}
          layout="vertical"
          requiredMark={false}
          onFinish={submit}
          onValuesChange={(changed) => {
            if ("product_id" in changed) {
              const product = products.find((item) => item.ID === changed.product_id);
              form.setFieldValue("unit_yuan", Number(centsToYuanInput(product?.DefaultPurchaseCents ?? 0)));
            }
          }}
        >
          {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
          <Form.Item label="商品" name="product_id" rules={[{ required: true, message: "请选择商品" }]}>
            <ProductCombobox products={selectableProducts} />
          </Form.Item>
          <Form.Item label="店铺（可选）" name="shop_id">
            <Select
              allowClear
              notFoundContent="没有店铺"
              options={shops.map((shop) => ({ value: shop.ID, label: shop.Name }))}
              placeholder="不选择店铺"
              showSearch
              optionFilterProp="label"
            />
          </Form.Item>
          <Flex gap={14} wrap>
            <Form.Item label="数量" name="quantity" rules={[{ required: true, message: "请输入数量" }]} style={{ flex: 1, minWidth: 180 }}>
              <InputNumber min={1} precision={0} style={{ width: "100%" }} />
            </Form.Item>
            <Form.Item label="进货单价（元）" name="unit_yuan" rules={[{ required: true, message: "请输入进货单价" }]} style={{ flex: 1, minWidth: 180 }}>
              <InputNumber min={0} precision={2} style={{ width: "100%" }} />
            </Form.Item>
          </Flex>
          <FormActions label="保存入库" saving={saving} onCancel={() => setOpen(false)} />
        </Form>
      </Modal>
    </>
  );
}

function OutboundForm({ products, shops, inventory, onDone }: Props) {
  const [form] = Form.useForm<OutboundValues>();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const selectableProducts = useSelectableProducts(products);
  const productID = Form.useWatch("product_id", form);
  const quantity = Form.useWatch("quantity", form) ?? 1;
  const stock = inventory.find((item) => item.ProductID === productID)?.Quantity ?? 0;
  const shortage = Boolean(productID) && quantity > stock;

  async function submit(values: OutboundValues) {
    setSaving(true);
    setError("");
    try {
      await apiPost("/inventory/sales-outbound", {
        product_id: values.product_id,
        shop_id: values.shop_id,
        quantity: values.quantity,
        sale_unit_cents: yuanToCents(String(values.sale_yuan ?? 0)),
      });
      setOpen(false);
      onDone("销售出库已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "出库失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Button icon={<ExportOutlined />} onClick={() => setOpen(true)}>销售出库</Button>
      <Modal
        destroyOnHidden
        footer={null}
        open={open}
        title="销售出库"
        onCancel={() => setOpen(false)}
        afterClose={() => { form.resetFields(); setError(""); }}
      >
        <Form<OutboundValues>
          form={form}
          initialValues={{ quantity: 1, sale_yuan: 0 }}
          layout="vertical"
          requiredMark={false}
          onFinish={submit}
          onValuesChange={(changed) => {
            if ("product_id" in changed) {
              const product = products.find((item) => item.ID === changed.product_id);
              form.setFieldValue("sale_yuan", Number(centsToYuanInput(product?.DefaultSaleCents ?? 0)));
            }
          }}
        >
          {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
          <Form.Item label="商品" name="product_id" rules={[{ required: true, message: "请选择商品" }]}>
            <ProductCombobox products={selectableProducts} />
          </Form.Item>
          <Form.Item label="店铺" name="shop_id" rules={[{ required: true, message: "请选择店铺" }]}>
            <Select
              notFoundContent="没有店铺"
              options={shops.map((shop) => ({ value: shop.ID, label: shop.Name }))}
              placeholder="选择店铺"
              showSearch
              optionFilterProp="label"
            />
          </Form.Item>
          <Flex gap={14} wrap>
            <Form.Item label={`数量（当前 ${formatQuantity(stock)}）`} name="quantity" rules={[{ required: true, message: "请输入数量" }]} style={{ flex: 1, minWidth: 180 }}>
              <InputNumber min={1} precision={0} style={{ width: "100%" }} />
            </Form.Item>
            <Form.Item label="销售单价（元）" name="sale_yuan" rules={[{ required: true, message: "请输入销售单价" }]} style={{ flex: 1, minWidth: 180 }}>
              <InputNumber min={0} precision={2} style={{ width: "100%" }} />
            </Form.Item>
          </Flex>
          {shortage ? <Alert message="当前库存不足，提交会被服务端拒绝。" showIcon style={{ marginBottom: 16 }} type="warning" /> : null}
          <FormActions label="保存出库" saving={saving} onCancel={() => setOpen(false)} />
        </Form>
      </Modal>
    </>
  );
}

function AdjustmentForm({ products, onDone }: Pick<Props, "products" | "onDone">) {
  const [form] = Form.useForm<AdjustmentValues>();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const selectableProducts = useSelectableProducts(products);

  async function submit(values: AdjustmentValues) {
    setSaving(true);
    setError("");
    try {
      await apiPost("/inventory/adjustments", {
        product_id: values.product_id,
        quantity_delta: values.quantity_delta,
        reason: values.reason,
      });
      setOpen(false);
      onDone("库存调整已记录");
    } catch (err) {
      setError(err instanceof Error ? err.message : "调整失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Button icon={<SlidersOutlined />} onClick={() => setOpen(true)}>调整</Button>
      <Modal
        destroyOnHidden
        footer={null}
        open={open}
        title="库存调整"
        onCancel={() => setOpen(false)}
        afterClose={() => { form.resetFields(); setError(""); }}
      >
        <Form<AdjustmentValues> form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
          <Form.Item label="商品" name="product_id" rules={[{ required: true, message: "请选择商品" }]}>
            <ProductCombobox products={selectableProducts} />
          </Form.Item>
          <Form.Item label="调整数量" name="quantity_delta" rules={[{ required: true, message: "请输入调整数量" }]} extra="正数增加，负数减少">
            <InputNumber precision={0} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item label="原因" name="reason" rules={[{ required: true, message: "请输入调整原因" }]}>
            <Input.TextArea maxLength={500} rows={3} showCount />
          </Form.Item>
          <FormActions label="保存调整" saving={saving} onCancel={() => setOpen(false)} />
        </Form>
      </Modal>
    </>
  );
}

function useSelectableProducts(products: Product[]) {
  return useMemo(() => products.filter((product) => product.Enabled && !product.ArchivedAt), [products]);
}

function FormActions({ saving, label, onCancel }: { saving: boolean; label: string; onCancel: () => void }) {
  return (
    <Flex gap={8} justify="flex-end">
      <Button onClick={onCancel}>取消</Button>
      <Button htmlType="submit" loading={saving} type="primary">{label}</Button>
    </Flex>
  );
}
