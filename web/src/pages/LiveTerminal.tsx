import { Empty } from '../components/StateView';

// SCR-06 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function LiveTerminal() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Live terminal</h1>
      <Empty title="Chưa có log" />
    </section>
  );
}
