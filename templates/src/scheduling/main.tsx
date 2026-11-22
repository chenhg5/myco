// Open directly in the personal calendar; booking setup is a secondary screen.
import "../_lib/sheet.css";
import "./scheduling.css";
import "./calendar.css";
import { useState } from "react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { kit } from "./model";
import { PersonalCalendar } from "./personal-calendar";
import { Appointments } from "./appointments";
function App() {
  useKit(kit);
  const [section, setSection] = useState("calendar");
  return <>{section === "calendar" ? <PersonalCalendar appointments={() => setSection("appointments")} /> : <Appointments onCalendar={() => setSection("calendar")} />}<Toast kit={kit} /></>;
}
void mount(kit, App);
