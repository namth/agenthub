import { Empty } from '../components/StateView';

// SCR-08 — khung tạm; nội dung thật làm ở task của màn hình này (docs/11).
export default function Office() {
  return (
    <section className="space-y-4">
      <h1 className="text-xl font-semibold">Văn phòng ảo</h1>
      <Empty title="Chưa có máy nào kết nối" />
    </section>
  );
}
