<!-- translated from docs/README.md at ba93a85f29e8 -->

# Hướng dẫn sử dụng tug

tug là một web framework cho các ứng dụng Go có frontend là
[Inertia.js](https://inertiajs.com): handler Go render các trang React, Vue
hoặc Svelte với props, không cần API ở giữa, và cả ứng dụng được đóng gói
thành một binary duy nhất. Hướng dẫn này đi từ một ứng dụng mới tạo đến khi
ứng dụng được triển khai. Tài liệu riêng của từng gói, trên
[pkg.go.dev](https://pkg.go.dev/github.com/cuonggt/tug), trình bày phần chi
tiết còn lại.

1. [Bắt đầu](getting-started.md): cài đặt tug, tạo một ứng dụng, chạy nó,
   rồi thêm một trang và một form.
2. [Định tuyến và handler](../routing.md): App, route, liên kết tuyệt đối
   và liên kết có chữ ký, nhóm route, middleware, trong đó có header bảo
   mật, route đã trả lời request, địa chỉ của client khi đứng sau proxy,
   `Ctx`, tệp, tải xuống, stream và sự kiện, bind request, và lỗi.
3. [Trang](../pages.md): trang Inertia và props của chúng, template gốc và
   nonce của nó, props được tính sau, props dùng chung, chuyển hướng, tải
   xuống và sự kiện trên một trang, trang lỗi, Vite và DevTools của
   Inertia.
4. [Render phía máy chủ](../ssr.md): trang được render trên máy chủ cho
   lượt truy cập đầu, bởi Node chạy cạnh ứng dụng, và gói `ssr`.
5. [Form và session](../forms.md): kiểm tra hợp lệ, quy tắc riêng của ứng
   dụng, form kiểm tra từng trường ngay khi người dùng rời khỏi nó, thông
   báo flash, session và CSRF.
6. [Đa ngôn ngữ](../languages.md): những gì tug và ứng dụng nói với người
   dùng, bằng ngôn ngữ của request, từ một tệp cho mỗi ngôn ngữ, gói
   `lang` và `tug lang`.
7. [Tệp](../files.md): tệp tải lên, được kiểm tra theo kích thước và loại
   thực sự của chúng, lưu trên đĩa của ứng dụng hoặc trong S3 bằng gói
   `storage`, và liên kết đến chúng, công khai hoặc có chữ ký.
8. [Tài khoản](../auth.md): starter auth, trên SQLite, Postgres hoặc MySQL,
   với API token, quản trị viên và thông báo của nó, cùng các gói `auth` và
   `mail`, có bản sao, tệp đính kèm và liên kết hủy đăng ký chỉ bằng một
   cú nhấp.
9. [Migration](../migrations.md): các bảng của cơ sở dữ liệu, được tạo và
   thay đổi bằng các tệp SQL, mỗi tệp chạy một lần, theo thứ tự, khi ứng
   dụng khởi động và qua lệnh `migrate` của ứng dụng, với
   `tug migrate new` để tạo migration tiếp theo, và gói `migrate`.
10. [Phân quyền](../authorization.md): người dùng được làm gì với một đối
    tượng, các ability của gói `auth`, lời từ chối thành một lỗi 403 nói
    rõ lý do, và những gì người dùng của trang được làm, có trong props
    của trang.
11. [Mã hóa](../encryption.md): khóa của ứng dụng, những gì tug mã hóa và
    ký bằng khóa đó, gói `crypt` cho các giá trị riêng của ứng dụng, và
    việc xoay vòng khóa.
12. [Tác vụ nền](../jobs.md): gói `queue`, cho những việc kéo dài hơn
    request, được chạy lại khi thất bại, chạy theo lịch, kể cả theo múi
    giờ, chỉ chờ chạy một lần dù được yêu cầu bao nhiêu lần, hoặc chỉ chạy
    một số lượng nhất định cùng lúc hay mỗi giây, trên mọi instance, và
    báo khi đã thất bại hẳn cũng như từng lần chạy diễn ra thế nào.
13. [Cache](../cache.md): gói `cache`, cho những gì tốn thời gian để tính,
    được lưu ở nơi mọi instance của ứng dụng đều tìm thấy, và lock cho
    những việc không được chạy hai lần cùng lúc.
14. [Phát sự kiện](../broadcasting.md): gói `broadcast`, sự kiện trên các
    kênh, được cơ sở dữ liệu của ứng dụng chuyển đến các trang đang mở trên
    mọi instance, để chúng tải lại phần đã thay đổi.
15. [TypeScript](../typescript.md): các kiểu mà `tug gen` sinh ra từ mã Go.
16. [Kiểm thử](../testing.md): gói `tugtest`, client của Inertia cho các
    test Go kiểm tra trang, form, tệp tải lên và đăng nhập của ứng dụng,
    `mailtest` cho email của ứng dụng, và các sự kiện của nó.
17. [Triển khai](../deployment.md): một binary, Dockerfile, biến môi
    trường, các header cho trình duyệt biết được làm gì với các trang của
    ứng dụng, trong đó có Content-Security-Policy, log và metric.
18. [CLI](../cli.md): `tug new`, `tug dev`, `tug gen`, `tug lang`,
    `tug migrate`, `tug build` và `tug key`, đầy đủ chi tiết.

[Lộ trình](../roadmap.md) kể lại tug đã được xây dựng ra sao, những quyết
định đằng sau nó và những gì sắp tới, còn
[các phép đo hiệu năng](../benchmarks.md) cho biết tug tốn bao nhiêu cho một
request và một ứng dụng làm bằng tug cần bao nhiêu để chạy, cùng cách đo
những con số đó.
