import { Empty } from '../components/StateView';

// SCR-02 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function Kanban() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Bảng Kanban</h1>
      <Empty title="Chưa có tác vụ nào trong hàng đợi" />
    </section>
  );
}
