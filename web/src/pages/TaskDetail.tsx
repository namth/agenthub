import { Empty } from '../components/StateView';

// SCR-04 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function TaskDetail() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Chi tiết task</h1>
      <Empty title="Chưa có dữ liệu task" />
    </section>
  );
}
