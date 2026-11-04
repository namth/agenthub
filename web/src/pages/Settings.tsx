import { Empty } from '../components/StateView';

// SCR-07 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function Settings() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Cài đặt hệ thống</h1>
      <Empty title="Cấu hình sẽ có ở TASK-17" />
    </section>
  );
}
