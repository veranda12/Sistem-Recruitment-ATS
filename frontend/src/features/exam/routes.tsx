import { Route, Routes } from "react-router";
import { ExamRoom } from "./ExamRoom";

// Bundle ruang ujian dipisah dari portal dan back office.
export default function ExamRoutes() {
  return (
    <Routes>
      <Route path=":id" element={<ExamRoom />} />
    </Routes>
  );
}
