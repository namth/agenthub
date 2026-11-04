import { Empty } from '../components/StateView';

// SCR-11 — khung tạm; nội dung thật làm ở task giao diện (FEAT-04/05).
export default function MyMachines() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Máy của tôi</h1>
      <Empty title="Đang xây dựng" />
    </section>
  );
}
