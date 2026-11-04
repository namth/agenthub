import { Empty } from '../components/StateView';

// SCR-03 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function TaskCreate() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Tạo yêu cầu mới</h1>
      <Empty title="Form tạo yêu cầu sẽ có ở TASK-06" />
    </section>
  );
}
